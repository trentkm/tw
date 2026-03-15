package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/trent/tmux-workspace/internal/notify"
	"github.com/trent/tmux-workspace/internal/tmux"
)

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true)

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6"))

	cursorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("4")).
			Foreground(lipgloss.Color("15")).
			Bold(true)

	currentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	windowActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7"))

	windowDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	notifBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")).
			Bold(true)

	notifMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

type sessionEntry struct {
	session tmux.Session
	windows []tmux.Window
	notif   *notify.Notification
}

type Model struct {
	entries        []sessionEntry
	cursor         int
	currentSession string
	width          int
	height         int
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func NewModel() Model {
	m := Model{
		currentSession: tmux.CurrentSession(),
	}
	m.loadSessions()
	return m
}

func (m *Model) loadSessions() {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return
	}

	entries := make([]sessionEntry, 0, len(sessions))
	for _, s := range sessions {
		wins, _ := tmux.ListWindows(s.Name)
		entries = append(entries, sessionEntry{
			session: s,
			windows: wins,
			notif:   notify.Get(s.Name),
		})
	}
	m.entries = entries

	// Keep cursor in bounds
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
}

func (m *Model) cursorToSession(name string) {
	for i, e := range m.entries {
		if e.session.Name == name {
			m.cursor = i
			return
		}
	}
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.loadSessions()
		return m, tickCmd()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			m.cursor = len(m.entries) - 1

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			m.cursor = 0

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(m.entries) > 0 {
				selected := m.entries[m.cursor].session.Name
				notify.Clear(selected)
				tmux.SwitchClient(selected)
				m.currentSession = selected
				m.loadSessions()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			if len(m.entries) > 0 {
				notify.Clear(m.entries[m.cursor].session.Name)
				m.loadSessions()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("C"))):
			notify.ClearAll()
			m.loadSessions()
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render(" WORKSPACES"))
	b.WriteString("\n")

	divWidth := m.width - 2
	if divWidth < 10 {
		divWidth = 20
	}
	b.WriteString(dividerStyle.Render(" " + strings.Repeat("─", divWidth)))
	b.WriteString("\n\n")

	// Sessions
	for idx, entry := range m.entries {
		name := entry.session.Name
		isCursor := idx == m.cursor
		isCurrent := name == m.currentSession

		// Session line
		var sessionLine string
		prefix := "  "
		if isCursor {
			prefix = "▸ "
		}

		label := prefix + name

		// Badge
		badge := ""
		if entry.notif != nil {
			badge = notifBadgeStyle.Render(fmt.Sprintf(" ● %s", entry.notif.TimeAgo()))
		}

		if isCursor {
			// Pad to full width for highlight bar
			padded := label
			remaining := m.width - lipgloss.Width(label) - lipgloss.Width(badge)
			if remaining > 0 {
				padded = label + strings.Repeat(" ", remaining)
			}
			if badge != "" {
				// Insert badge before padding end
				remaining = m.width - lipgloss.Width(label) - lipgloss.Width(badge)
				if remaining > 0 {
					padded = label + badge + strings.Repeat(" ", remaining)
				} else {
					padded = label + badge
				}
			}
			sessionLine = cursorStyle.Render(padded)
		} else if isCurrent {
			sessionLine = currentStyle.Render(label) + badge
		} else {
			sessionLine = normalStyle.Render(label) + badge
		}

		b.WriteString(sessionLine)
		b.WriteString("\n")

		// Windows
		for _, win := range entry.windows {
			info := shortenPath(win.Path)
			if win.Command != "fish" && win.Command != "bash" && win.Command != "zsh" {
				info = win.Command
			}

			var wline string
			if win.Active {
				wline = windowActiveStyle.Render(fmt.Sprintf("    › %s ", win.Name)) +
					windowDimStyle.Render(info)
			} else {
				wline = windowDimStyle.Render(fmt.Sprintf("      %s %s", win.Name, info))
			}
			b.WriteString(wline)
			b.WriteString("\n")
		}

		// Notification message
		if entry.notif != nil {
			b.WriteString(notifMsgStyle.Render(fmt.Sprintf("     ↳ %s", entry.notif.Message)))
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	// Footer
	footer := footerStyle.Render(fmt.Sprintf(
		" %s\n %s\n %s\n %s",
		strings.Repeat("─", divWidth),
		"j/k navigate",
		"enter switch session",
		"c clear notif  q close",
	))
	b.WriteString(footer)

	return b.String()
}

func shortenPath(path string) string {
	home, _ := os.UserHomeDir()
	path = strings.Replace(path, home, "~", 1)
	return filepath.Base(path)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
