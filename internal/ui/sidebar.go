package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/trent/tmux-workspace/internal/notify"
	"github.com/trent/tmux-workspace/internal/tmux"
)

// ── Palette ──────────────────────────────────────────────────────────
var (
	colorAccent  = lipgloss.Color("4")  // blue  — current session
	colorHeader  = lipgloss.Color("6")  // cyan  — chrome
	colorText    = lipgloss.Color("7")  // light — primary text
	colorMuted   = lipgloss.Color("8")  // gray  — secondary info
	colorBright  = lipgloss.Color("15") // white — emphasis
	colorNotif   = lipgloss.Color("3")  // yellow — needs attention
	colorWorking = lipgloss.Color("6")  // cyan  — agent working
	colorDone    = lipgloss.Color("2")  // green — task done
	colorCursorB = lipgloss.Color("8")  // cursor background
)

// ── Styles ───────────────────────────────────────────────────────────
var (
	cursorStyle = lipgloss.NewStyle().
			Background(colorCursorB).
			Foreground(colorBright).
			Bold(true)

	cursorCurrentStyle = lipgloss.NewStyle().
				Background(colorAccent).
				Foreground(colorBright).
				Bold(true)

	currentStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(colorText)

	detailStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	statusAttentionStyle = lipgloss.NewStyle().
				Foreground(colorNotif).
				Bold(true)

	statusWorkingStyle = lipgloss.NewStyle().
				Foreground(colorWorking)

	statusDoneStyle = lipgloss.NewStyle().
			Foreground(colorDone)

	footerKeyStyle = lipgloss.NewStyle().
			Foreground(colorHeader).
			Bold(true)

	footerDescStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	emptyStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)

// ── Model ────────────────────────────────────────────────────────────

type sessionEntry struct {
	session     tmux.Session
	windows     []tmux.Window
	notif       *notify.Notification
	simple      bool            // collapsible to one line
	path        string          // primary path for simple sessions
	agentName   string          // detected agent name (e.g. "claude")
	agentStatus tmux.AgentStatus // working, idle, or none
}

type Model struct {
	entries        []sessionEntry
	cursor         int
	currentSession string
	viewport       viewport.Model
	width          int
	height         int
	ready          bool
	cmdMode        bool   // true when typing a : command
	cmdBuf         string // command buffer after :
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func NewModel() Model {
	m := Model{
		currentSession: tmux.ClientSession(),
	}
	m.loadSessions()
	return m
}

func (m *Model) loadSessions() {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return
	}

	m.currentSession = tmux.ClientSession()

	entries := make([]sessionEntry, 0, len(sessions))
	for _, s := range sessions {
		wins, _ := tmux.ListWindowsWithPanes(s.Name)
		agentName, agentStatus := tmux.SessionAgentStatus(wins)
		entry := sessionEntry{
			session:     s,
			windows:     wins,
			notif:       notify.Get(s.Name),
			agentName:   agentName,
			agentStatus: agentStatus,
		}
		classifyEntry(&entry)
		entries = append(entries, entry)
	}
	m.entries = entries

	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
}

// classifyEntry determines if a session can be collapsed to one line.
func classifyEntry(e *sessionEntry) {
	if len(e.windows) != 1 {
		e.simple = false
		return
	}
	win := e.windows[0]
	nonShell := 0
	for _, p := range win.Panes {
		if !tmux.IsShell(p.Command) {
			nonShell++
		}
	}
	if nonShell <= 1 {
		e.simple = true
		e.path = tildefy(windowPath(win))
		return
	}
	e.simple = false
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// ── Update ───────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := m.height - chromeHeight
		if contentHeight < 1 {
			contentHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, contentHeight)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = contentHeight
		}
		m.viewport.SetContent(m.renderSessions())
		return m, nil

	case tickMsg:
		m.loadSessions()
		if m.ready {
			m.viewport.SetContent(m.renderSessions())
		}
		return m, tickCmd()

	case tea.KeyMsg:
		// Command mode (:q, :qa, etc.)
		if m.cmdMode {
			switch msg.String() {
			case "enter":
				cmd := m.cmdBuf
				m.cmdMode = false
				m.cmdBuf = ""
				switch cmd {
				case "q", "qa", "q!", "qa!":
					return m, tea.Quit
				}
			case "esc":
				m.cmdMode = false
				m.cmdBuf = ""
			case "backspace":
				if len(m.cmdBuf) > 0 {
					m.cmdBuf = m.cmdBuf[:len(m.cmdBuf)-1]
				} else {
					m.cmdMode = false
				}
			default:
				m.cmdBuf += msg.String()
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys(":"))):
			m.cmdMode = true
			m.cmdBuf = ""

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.viewport.SetContent(m.renderSessions())
				m.ensureCursorVisible()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			if m.cursor > 0 {
				m.cursor--
				m.viewport.SetContent(m.renderSessions())
				m.ensureCursorVisible()
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			m.cursor = len(m.entries) - 1
			m.viewport.SetContent(m.renderSessions())
			m.viewport.GotoBottom()

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			m.cursor = 0
			m.viewport.SetContent(m.renderSessions())
			m.viewport.GotoTop()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(m.entries) > 0 {
				selected := m.entries[m.cursor].session.Name
				tmux.SwitchClient(selected)
				return m, tea.Quit // dismiss popup after switching
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			if len(m.entries) > 0 {
				notify.Clear(m.entries[m.cursor].session.Name)
				m.loadSessions()
				m.viewport.SetContent(m.renderSessions())
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("C"))):
			notify.ClearAll()
			m.loadSessions()
			m.viewport.SetContent(m.renderSessions())
		}
	}
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────

// chromeHeight is fixed to avoid circular rendering. footer(1) + gap(1) = 2
// The summary line is part of the viewport content, so it doesn't add to chrome.
const chromeHeight = 2

func (m Model) View() string {
	if !m.ready {
		return ""
	}

	var b strings.Builder
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m Model) renderFooter() string {
	if m.cmdMode {
		return " :" + m.cmdBuf + "█"
	}
	keys := []struct{ key, desc string }{
		{"j/k", "navigate"},
		{"enter", "switch"},
		{"c", "clear"},
		{"esc", "close"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts,
			footerKeyStyle.Render(k.key)+" "+footerDescStyle.Render(k.desc))
	}
	return " " + strings.Join(parts, detailStyle.Render("  "))
}

// ── Session list (viewport content) ─────────────────────────────────

func (m Model) renderSessions() string {
	if len(m.entries) == 0 {
		return emptyStyle.Render("\n  No sessions running.\n")
	}

	var b strings.Builder
	w := m.contentWidth()

	// Aggregate agent status summary
	if summary := m.agentSummary(); summary != "" {
		b.WriteString(" " + summary + "\n\n")
	}

	for idx, entry := range m.entries {
		isCursor := idx == m.cursor
		isCurrent := entry.session.Name == m.currentSession
		isLast := idx == len(m.entries)-1

		if entry.simple {
			b.WriteString(m.renderSimpleSession(entry, isCursor, isCurrent, w))
			b.WriteString("\n")
		} else {
			b.WriteString(m.renderSessionLine(entry.session.Name, isCursor, isCurrent, entry, "", w))
			b.WriteString("\n")
			b.WriteString(m.renderTree(entry))
		}

		if !isLast {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderSimpleSession(entry sessionEntry, isCursor, isCurrent bool, w int) string {
	suffix := entry.path
	// Don't show agent name inline if we're already showing agent status in the badge
	if entry.agentStatus == tmux.AgentNone && len(entry.windows) == 1 {
		for _, p := range entry.windows[0].Panes {
			if !tmux.IsShell(p.Command) {
				suffix = entry.path + "  " + friendlyName(p)
				break
			}
		}
	}
	return m.renderSessionLine(entry.session.Name, isCursor, isCurrent, entry, suffix, w)
}

// statusBadge returns a styled status indicator for a session.
func statusBadge(entry sessionEntry) string {
	// Priority: notification > live agent detection
	if entry.notif != nil {
		ago := entry.notif.TimeAgo()
		switch entry.notif.Status {
		case notify.StatusWaiting:
			return statusAttentionStyle.Render("● agent waiting ") + detailStyle.Render(ago)
		case notify.StatusDone:
			return statusDoneStyle.Render("✓ agent done ") + detailStyle.Render(ago)
		}
	}
	if entry.agentStatus == tmux.AgentWorking {
		return statusWorkingStyle.Render("⟳ agent working")
	}
	return ""
}

func (m Model) renderSessionLine(name string, isCursor, isCurrent bool, entry sessionEntry, detail string, w int) string {
	var prefix string
	switch {
	case isCursor && isCurrent:
		prefix = "◆ "
	case isCursor:
		prefix = "▸ "
	case isCurrent:
		prefix = "◆ "
	default:
		prefix = "  "
	}
	label := prefix + name

	detailSuffix := ""
	if detail != "" {
		detailSuffix = "  " + detail
	}

	badge := statusBadge(entry)

	// Build content: label + detail + badge (always left-aligned together)
	content := label + detailSuffix
	if badge != "" {
		content += "  " + badge
	}

	if isCursor {
		style := cursorStyle
		if isCurrent {
			style = cursorCurrentStyle
		}
		// Pad to full width for the highlight bar, but badge stays with text
		contentWidth := lipgloss.Width(content)
		padding := w + 2 - contentWidth
		if padding < 0 {
			padding = 0
		}
		return style.Render(content + strings.Repeat(" ", padding))
	}

	var line string
	if isCurrent {
		line = currentStyle.Render(label)
	} else {
		line = normalStyle.Render(label)
	}
	if detailSuffix != "" {
		line += detailStyle.Render(detailSuffix)
	}
	if badge != "" {
		line += "  " + badge
	}
	return line
}

func (m Model) renderTree(entry sessionEntry) string {
	var b strings.Builder

	for wi, win := range entry.windows {
		isLastWin := wi == len(entry.windows)-1 && entry.notif == nil

		winConn := "├─"
		if isLastWin {
			winConn = "└─"
		}
		childPrefix := "│  "
		if isLastWin {
			childPrefix = "   "
		}

		winPath := windowPath(win)
		b.WriteString(detailStyle.Render("    " + winConn + " " + winPath))
		b.WriteString("\n")

		// Show pane sub-tree only when there are multiple non-shell panes
		nonShellPanes := filterNonShell(win.Panes)
		if len(nonShellPanes) > 1 {
			for pi, pane := range nonShellPanes {
				paneConn := "├─"
				if pi == len(nonShellPanes)-1 {
					paneConn = "└─"
				}
				b.WriteString(detailStyle.Render(fmt.Sprintf("    %s %s %s", childPrefix, paneConn, friendlyName(pane))))
				b.WriteString("\n")
			}
		} else if len(nonShellPanes) == 1 {
			b.WriteString(detailStyle.Render(fmt.Sprintf("    %s └─ %s", childPrefix, friendlyName(nonShellPanes[0]))))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// ── Helpers ──────────────────────────────────────────────────────────

func (m Model) agentSummary() string {
	var working, waiting, done int
	for _, e := range m.entries {
		if e.notif != nil {
			switch e.notif.Status {
			case notify.StatusWaiting:
				waiting++
			case notify.StatusDone:
				done++
			}
		} else if e.agentStatus == tmux.AgentWorking {
			working++
		}
	}

	var parts []string
	if waiting > 0 {
		parts = append(parts, statusAttentionStyle.Render(fmt.Sprintf("%d waiting", waiting)))
	}
	if working > 0 {
		parts = append(parts, statusWorkingStyle.Render(fmt.Sprintf("%d working", working)))
	}
	if done > 0 {
		parts = append(parts, statusDoneStyle.Render(fmt.Sprintf("%d done", done)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, detailStyle.Render(" · "))
}

func filterNonShell(panes []tmux.Pane) []tmux.Pane {
	var result []tmux.Pane
	for _, p := range panes {
		if !tmux.IsShell(p.Command) {
			result = append(result, p)
		}
	}
	return result
}

func windowPath(w tmux.Window) string {
	for _, p := range w.Panes {
		if p.Active {
			return tildefy(p.Path)
		}
	}
	if len(w.Panes) > 0 {
		return tildefy(w.Panes[0].Path)
	}
	return ""
}

func friendlyName(p tmux.Pane) string {
	if strings.Contains(p.Title, "Claude Code") {
		return "claude"
	}
	return p.Command
}

func (m Model) contentWidth() int {
	w := m.width - 2
	if w < 10 {
		w = 20
	}
	return w
}

func (m *Model) ensureCursorVisible() {
	line := 0
	for i := 0; i < m.cursor && i < len(m.entries); i++ {
		line += m.entryHeight(m.entries[i])
		line++ // blank separator
	}

	vpTop := m.viewport.YOffset
	vpBottom := vpTop + m.viewport.Height

	if line < vpTop {
		m.viewport.SetYOffset(line)
	} else if line >= vpBottom {
		m.viewport.SetYOffset(line - m.viewport.Height + m.entryHeight(m.entries[m.cursor]))
	}
}

func (m Model) entryHeight(e sessionEntry) int {
	if e.simple {
		return 1
	}
	h := 1 // session line
	for _, w := range e.windows {
		h++ // window line
		nonShell := filterNonShell(w.Panes)
		h += len(nonShell)
	}
	return h
}

func tildefy(path string) string {
	home, _ := os.UserHomeDir()
	if realHome, err := filepath.EvalSymlinks(home); err == nil && realHome != home {
		path = strings.Replace(path, realHome, "~", 1)
	}
	path = strings.Replace(path, home, "~", 1)

	entries, _ := os.ReadDir(home)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			link := filepath.Join(home, e.Name())
			if target, err := filepath.EvalSymlinks(link); err == nil {
				path = strings.Replace(path, target, "~/"+e.Name(), 1)
			}
		}
	}
	return path
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
