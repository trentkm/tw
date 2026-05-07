package gateway

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/trentkm/agmux/internal/notify"
	"github.com/trentkm/agmux/internal/tmux"
)

func listSessionNames() ([]string, error) {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}
	return names, nil
}

func sessionExists(name string) bool {
	sessions, err := listSessionNames()
	if err != nil {
		return false
	}
	for _, s := range sessions {
		if s == name {
			return true
		}
	}
	return false
}

func sendToPane(session, msg string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, msg, "Enter")
	return cmd.Run()
}

func sendCtrlC(session string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, "C-c")
	return cmd.Run()
}

func capturePane(session string, lines int) (string, error) {
	arg := fmt.Sprintf("-%d", lines)
	out, err := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", arg).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isSessionIdle(session string) bool {
	n := notify.Get(session)
	if n != nil && (n.Status == notify.StatusDone || n.Status == notify.StatusWaiting) {
		return true
	}

	// Fallback: check if agent is detected as idle via tmux
	wins, err := tmux.ListWindowsWithPanes(session)
	if err != nil {
		return false
	}
	tmux.ResetProcessTree()
	_, status := tmux.SessionAgentStatus(wins)
	return status == tmux.AgentIdle || status == tmux.AgentNone
}

func isSessionWaiting(session string) bool {
	n := notify.Get(session)
	return n != nil && n.Status == notify.StatusWaiting
}

func formatStatus() string {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return "No tmux sessions found."
	}

	var lines []string
	for _, s := range sessions {
		wins, _ := tmux.ListWindowsWithPanes(s.Name)
		tmux.ResetProcessTree()
		agentName, agentStatus := tmux.SessionAgentStatus(wins)
		n := notify.Get(s.Name)

		status := "idle"
		switch {
		case agentStatus == tmux.AgentWorking:
			status = "working"
		case n != nil && n.Status == notify.StatusWorking:
			status = "working"
		case n != nil && n.Status == notify.StatusWaiting:
			status = "waiting"
		case n != nil && n.Status == notify.StatusDone:
			status = "done"
		}

		agent := agentName
		if agent == "" {
			agent = "-"
		}

		lines = append(lines, fmt.Sprintf("%-12s %-8s %s", s.Name, status, agent))
	}

	if len(lines) == 0 {
		return "No sessions found."
	}

	header := fmt.Sprintf("%-12s %-8s %s", "SESSION", "STATUS", "AGENT")
	return header + "\n" + strings.Join(lines, "\n")
}
