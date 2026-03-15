package notify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	StatusWaiting Status = "waiting" // agent needs user attention
	StatusDone    Status = "done"    // agent finished its task
)

type Notification struct {
	Session   string
	Status    Status
	Timestamp time.Time
}

func stateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "tmux-workspace")
	os.MkdirAll(dir, 0755)
	return dir
}

func Add(session string, status Status) error {
	ts := time.Now().Unix()
	data := fmt.Sprintf("%d|%s", ts, status)
	path := filepath.Join(stateDir(), session+".notify")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return err
	}

	// Fire macOS notification. Clicking it switches to the session.
	var msg string
	switch status {
	case StatusWaiting:
		msg = "Agent waiting for response"
	case StatusDone:
		msg = "Agent finished task"
	}

	// Prefer terminal-notifier (supports click-to-switch), fall back to osascript
	// Use full paths since terminal-notifier runs outside the user's shell
	tmuxPath, _ := exec.LookPath("tmux")
	if tmuxPath == "" {
		tmuxPath = "/opt/homebrew/bin/tmux"
	}
	switchCmd := fmt.Sprintf("%s switch-client -t %s", tmuxPath, session)
	if path, err := exec.LookPath("terminal-notifier"); err == nil {
		exec.Command(path,
			"-title", session,
			"-message", msg,
			"-sound", "Glass",
			"-execute", switchCmd,
			"-group", "tw-"+session,
		).Run()
	} else {
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "%s" with title "%s" sound name "Glass"`, msg, session),
		).Run()
	}

	return nil
}

func Clear(session string) error {
	path := filepath.Join(stateDir(), session+".notify")
	return os.Remove(path)
}

func ClearAll() error {
	matches, _ := filepath.Glob(filepath.Join(stateDir(), "*.notify"))
	for _, f := range matches {
		os.Remove(f)
	}
	return nil
}

func Get(session string) *Notification {
	path := filepath.Join(stateDir(), session+".notify")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 2)
	if len(parts) != 2 {
		return nil
	}
	ts, _ := strconv.ParseInt(parts[0], 10, 64)
	return &Notification{
		Session:   session,
		Status:    Status(parts[1]),
		Timestamp: time.Unix(ts, 0),
	}
}

func Count() int {
	matches, _ := filepath.Glob(filepath.Join(stateDir(), "*.notify"))
	return len(matches)
}

func CountByStatus(status Status) int {
	matches, _ := filepath.Glob(filepath.Join(stateDir(), "*.notify"))
	count := 0
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 2)
		if len(parts) == 2 && Status(parts[1]) == status {
			count++
		}
	}
	return count
}

func (n *Notification) TimeAgo() string {
	ago := time.Since(n.Timestamp)
	mins := int(ago.Minutes())
	if mins == 0 {
		return "now"
	}
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh", mins/60)
}
