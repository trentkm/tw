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

type Notification struct {
	Session   string
	Message   string
	Timestamp time.Time
}

func stateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "state", "tmux-workspace")
	os.MkdirAll(dir, 0755)
	return dir
}

func Add(session, message string) error {
	ts := time.Now().Unix()
	data := fmt.Sprintf("%d|%s", ts, message)
	path := filepath.Join(stateDir(), session+".notify")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return err
	}

	// Fire macOS notification in background
	go func() {
		exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification "%s" with title "%s" sound name "Glass"`, message, session),
		).Run()
	}()

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

func List() []Notification {
	matches, _ := filepath.Glob(filepath.Join(stateDir(), "*.notify"))
	var notifs []Notification
	for _, f := range matches {
		session := strings.TrimSuffix(filepath.Base(f), ".notify")
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 2)
		if len(parts) != 2 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[0], 10, 64)
		notifs = append(notifs, Notification{
			Session:   session,
			Message:   parts[1],
			Timestamp: time.Unix(ts, 0),
		})
	}
	return notifs
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
		Message:   parts[1],
		Timestamp: time.Unix(ts, 0),
	}
}

func Count() int {
	matches, _ := filepath.Glob(filepath.Join(stateDir(), "*.notify"))
	return len(matches)
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
