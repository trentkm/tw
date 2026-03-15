package tmux

import (
	"os/exec"
	"strconv"
	"strings"
)

type Session struct {
	Name     string
	Windows  int
	Attached int
}

type Window struct {
	Index  int
	Name   string
	Active bool
	Panes  []Pane
}

type Pane struct {
	Index   int
	Command string
	Path    string
	Title   string
	Active  bool
}

func Run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func CurrentSession() string {
	out, err := Run("display-message", "-p", "#S")
	if err != nil {
		return ""
	}
	return out
}

// ClientSession returns the session the most recently active client is viewing.
// This differs from CurrentSession() when the sidebar pane belongs to a
// different session than the one the user is looking at (after switch-client).
func ClientSession() string {
	out, err := Run("list-clients", "-F", "#{client_activity}|#{client_session}")
	if err != nil {
		return CurrentSession()
	}
	// Find most recently active client
	var bestSession string
	var bestActivity string
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] > bestActivity {
			bestActivity = parts[0]
			bestSession = parts[1]
		}
	}
	if bestSession == "" {
		return CurrentSession()
	}
	return bestSession
}

func ListSessions() ([]Session, error) {
	out, err := Run("list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}")
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		wins, _ := strconv.Atoi(parts[1])
		attached, _ := strconv.Atoi(parts[2])
		sessions = append(sessions, Session{
			Name:     parts[0],
			Windows:  wins,
			Attached: attached,
		})
	}
	return sessions, nil
}

// ListWindowsWithPanes returns all windows for a session, each with their panes.
func ListWindowsWithPanes(session string) ([]Window, error) {
	out, err := Run("list-panes", "-s", "-t", session, "-F",
		"#{window_index}|#{window_name}|#{window_active}|#{pane_index}|#{pane_current_command}|#{pane_current_path}|#{pane_title}|#{pane_active}")
	if err != nil {
		return nil, err
	}

	windowMap := make(map[int]*Window)
	var windowOrder []int

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 8)
		if len(parts) != 8 {
			continue
		}
		winIdx, _ := strconv.Atoi(parts[0])
		paneIdx, _ := strconv.Atoi(parts[3])

		w, ok := windowMap[winIdx]
		if !ok {
			w = &Window{
				Index:  winIdx,
				Name:   parts[1],
				Active: parts[2] == "1",
			}
			windowMap[winIdx] = w
			windowOrder = append(windowOrder, winIdx)
		}

		w.Panes = append(w.Panes, Pane{
			Index:   paneIdx,
			Command: parts[4],
			Path:    parts[5],
			Title:   parts[6],
			Active:  parts[7] == "1",
		})
	}

	var windows []Window
	for _, idx := range windowOrder {
		windows = append(windows, *windowMap[idx])
	}
	return windows, nil
}

func SwitchClient(session string) error {
	_, err := Run("switch-client", "-t", session)
	return err
}

func SplitWindow(args ...string) error {
	a := append([]string{"split-window"}, args...)
	_, err := Run(a...)
	return err
}

func KillPane(paneID string) error {
	_, err := Run("kill-pane", "-t", paneID)
	return err
}

// IsShell returns true if the command is a common shell.
func IsShell(cmd string) bool {
	switch cmd {
	case "fish", "bash", "zsh", "sh", "dash", "tcsh", "csh":
		return true
	}
	return false
}

// AgentStatus represents the state of an AI agent in a pane.
type AgentStatus int

const (
	AgentNone    AgentStatus = iota // no agent detected
	AgentWorking                    // agent is actively processing
	AgentIdle                       // agent is idle, waiting for input
)

// knownAgents maps patterns (checked against pane title and command) to
// friendly display names. Ordered so title matches are checked first.
var knownAgents = []struct {
	pattern string
	name    string
}{
	// Title patterns (Claude Code is the only one that sets pane titles)
	{"Claude Code", "claude"},
	// Command patterns (binary names as they appear in pane_current_command)
	{"codex", "codex"},
	{"kiro-cli", "kiro"},
	{"kiro", "kiro"},
	{"aider", "aider"},
	{"goose", "goose"},
}

// hasBrailleSpinner checks if a string contains braille spinner characters
// (U+2800 block), used by Claude Code to indicate active processing.
func hasBrailleSpinner(s string) bool {
	for _, r := range s {
		if r >= 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}

// DetectAgent checks a pane's title and command to determine if an AI agent
// is running and whether it's actively working or idle.
// Spinner-based working detection only works for agents that set pane titles
// (currently only Claude Code). For others, we can detect presence but not
// working vs idle.
func DetectAgent(p Pane) (name string, status AgentStatus) {
	for _, agent := range knownAgents {
		if strings.Contains(p.Title, agent.pattern) || strings.Contains(p.Command, agent.pattern) {
			if hasBrailleSpinner(p.Title) {
				return agent.name, AgentWorking
			}
			return agent.name, AgentIdle
		}
	}
	return "", AgentNone
}

// SessionAgentStatus returns the aggregate agent status for a session
// by checking all panes. Working beats Idle.
func SessionAgentStatus(windows []Window) (name string, status AgentStatus) {
	for _, w := range windows {
		for _, p := range w.Panes {
			n, s := DetectAgent(p)
			if s == AgentWorking {
				return n, AgentWorking
			}
			if s == AgentIdle && status != AgentWorking {
				name = n
				status = s
			}
		}
	}
	return name, status
}
