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
	Index   int
	Name    string
	Active  bool
	Command string
	Path    string
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

func ListWindows(session string) ([]Window, error) {
	out, err := Run("list-windows", "-t", session, "-F",
		"#{window_index}|#{window_name}|#{window_active}|#{pane_current_command}|#{pane_current_path}")
	if err != nil {
		return nil, err
	}

	var windows []Window
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		idx, _ := strconv.Atoi(parts[0])
		windows = append(windows, Window{
			Index:   idx,
			Name:    parts[1],
			Active:  parts[2] == "1",
			Command: parts[3],
			Path:    parts[4],
		})
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
