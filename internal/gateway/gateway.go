package gateway

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Gateway struct {
	client  *slack.Client
	socket  *socketmode.Client
	cfg     *Config
	threads map[string]string // thread_ts → session name
	mu      sync.Mutex
	logger  *log.Logger
}

func New(cfg *Config, logger *log.Logger) *Gateway {
	client := slack.New(
		cfg.SlackBotToken,
		slack.OptionAppLevelToken(cfg.SlackAppToken),
	)
	socket := socketmode.New(client,
		socketmode.OptionLog(log.New(logger.Writer(), "socketmode: ", log.Lmsgprefix)),
	)

	return &Gateway{
		client:  client,
		socket:  socket,
		cfg:     cfg,
		threads: make(map[string]string),
		logger:  logger,
	}
}

func (g *Gateway) Run(ctx context.Context) error {
	go g.handleEvents(ctx)
	g.logger.Println("gateway started, listening for Slack messages...")
	return g.socket.RunContext(ctx)
}

func (g *Gateway) handleEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-g.socket.Events:
			if !ok {
				return
			}
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				g.socket.Ack(*evt.Request)
				eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				g.handleEventsAPI(eventsAPI)
			case socketmode.EventTypeConnecting:
				g.logger.Println("connecting to Slack...")
			case socketmode.EventTypeConnected:
				g.logger.Println("connected to Slack")
			case socketmode.EventTypeConnectionError:
				g.logger.Println("connection error, retrying...")
			}
		}
	}
}

func (g *Gateway) handleEventsAPI(evt slackevents.EventsAPIEvent) {
	switch evt.Type {
	case slackevents.CallbackEvent:
		switch innerEvt := evt.InnerEvent.Data.(type) {
		case *slackevents.MessageEvent:
			g.handleMessage(innerEvt)
		}
	}
}

func (g *Gateway) handleMessage(ev *slackevents.MessageEvent) {
	// Ignore bot messages, edits, and non-owner messages
	if ev.User != g.cfg.OwnerID || ev.BotID != "" || ev.SubType != "" {
		return
	}

	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}

	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp
	}

	// Handle slash commands
	if g.handleCommand(ev, text, threadTS) {
		return
	}

	// Resolve target session
	session, msg := g.resolveSession(text, threadTS)
	if session == "" {
		names, _ := listSessionNames()
		reply := "Which session? Active sessions:\n```\n" + strings.Join(names, "\n") + "\n```"
		g.reply(ev.Channel, threadTS, reply)
		return
	}

	// Send to tmux
	if err := sendToPane(session, msg); err != nil {
		g.reply(ev.Channel, threadTS, fmt.Sprintf("Failed to send to `%s`: %v", session, err))
		return
	}

	// Acknowledge with eyes reaction
	g.client.AddReaction("eyes", slack.ItemRef{
		Channel:   ev.Channel,
		Timestamp: ev.TimeStamp,
	})

	// Poll for completion in background
	go g.waitAndReply(ev.Channel, threadTS, ev.TimeStamp, session)
}

func (g *Gateway) handleCommand(ev *slackevents.MessageEvent, text, threadTS string) bool {
	parts := strings.SplitN(text, " ", 3)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "status":
		g.reply(ev.Channel, threadTS, "```\n"+formatStatus()+"\n```")
		return true

	case "peek":
		if len(parts) < 2 {
			g.reply(ev.Channel, threadTS, "Usage: `peek <session>`")
			return true
		}
		session := parts[1]
		if !sessionExists(session) {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Session `%s` not found.", session))
			return true
		}
		out, err := capturePane(session, g.cfg.CaptureLines)
		if err != nil {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Failed to capture `%s`: %v", session, err))
			return true
		}
		g.reply(ev.Channel, threadTS, formatOutput(session, out))
		return true

	case "attach":
		if len(parts) < 2 {
			g.reply(ev.Channel, threadTS, "Usage: `attach <session>`")
			return true
		}
		session := parts[1]
		if !sessionExists(session) {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Session `%s` not found.", session))
			return true
		}
		out, err := capturePane(session, 200)
		if err != nil {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Failed to capture `%s`: %v", session, err))
			return true
		}
		g.reply(ev.Channel, threadTS, formatOutput(session, out))
		return true

	case "send":
		if len(parts) < 3 {
			g.reply(ev.Channel, threadTS, "Usage: `send <session> <message>`")
			return true
		}
		session := parts[1]
		msg := parts[2]
		if !sessionExists(session) {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Session `%s` not found.", session))
			return true
		}
		if err := sendToPane(session, msg); err != nil {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Failed to send to `%s`: %v", session, err))
			return true
		}
		g.client.AddReaction("thumbsup", slack.ItemRef{
			Channel:   ev.Channel,
			Timestamp: ev.TimeStamp,
		})
		return true

	case "cancel":
		if len(parts) < 2 {
			g.reply(ev.Channel, threadTS, "Usage: `cancel <session>`")
			return true
		}
		session := parts[1]
		if !sessionExists(session) {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Session `%s` not found.", session))
			return true
		}
		if err := sendCtrlC(session); err != nil {
			g.reply(ev.Channel, threadTS, fmt.Sprintf("Failed to cancel `%s`: %v", session, err))
			return true
		}
		g.client.AddReaction("octagonal_sign", slack.ItemRef{
			Channel:   ev.Channel,
			Timestamp: ev.TimeStamp,
		})
		return true
	}

	return false
}

func (g *Gateway) resolveSession(text, threadTS string) (session, msg string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If thread already mapped, reuse
	if s, ok := g.threads[threadTS]; ok {
		return s, text
	}

	// Check if message starts with a session name
	parts := strings.SplitN(text, " ", 2)
	if sessionExists(parts[0]) {
		g.threads[threadTS] = parts[0]
		if len(parts) > 1 {
			return parts[0], parts[1]
		}
		return parts[0], ""
	}

	// If only one session exists, default to it
	names, err := listSessionNames()
	if err != nil || len(names) == 0 {
		return "", ""
	}
	if len(names) == 1 {
		g.threads[threadTS] = names[0]
		return names[0], text
	}

	return "", ""
}

func (g *Gateway) waitAndReply(channel, threadTS, msgTS, session string) {
	deadline := time.Now().Add(g.cfg.IdleTimeout)

	// Brief delay to let the agent pick up the message
	time.Sleep(3 * time.Second)

	// Check if agent is waiting (permission prompt) — forward immediately
	if isSessionWaiting(session) {
		out, _ := capturePane(session, 30)
		g.reply(channel, threadTS, formatOutput(session, out)+"\n_Agent is waiting for input._")
		return
	}

	lastUpdate := time.Now()
	for time.Now().Before(deadline) {
		if isSessionIdle(session) {
			// Remove the eyes reaction, add checkmark
			g.client.RemoveReaction("eyes", slack.ItemRef{Channel: channel, Timestamp: msgTS})
			g.client.AddReaction("white_check_mark", slack.ItemRef{Channel: channel, Timestamp: msgTS})

			out, _ := capturePane(session, g.cfg.CaptureLines)
			g.reply(channel, threadTS, formatOutput(session, out))
			return
		}

		// Send periodic "still working" update every 60s
		if time.Since(lastUpdate) > 60*time.Second {
			out, _ := capturePane(session, 20)
			preview := extractLastResponse(out)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			g.reply(channel, threadTS, fmt.Sprintf("_Still working on %s..._\n```\n%s\n```", session, preview))
			lastUpdate = time.Now()
		}

		time.Sleep(g.cfg.IdlePollInterval)
	}

	// Timeout
	g.client.RemoveReaction("eyes", slack.ItemRef{Channel: channel, Timestamp: msgTS})
	g.client.AddReaction("hourglass", slack.ItemRef{Channel: channel, Timestamp: msgTS})
	g.reply(channel, threadTS, fmt.Sprintf("Timed out waiting for `%s` to finish. Use `peek %s` to check.", session, session))
}

func (g *Gateway) reply(channel, threadTS, text string) {
	g.client.PostMessage(channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
}
