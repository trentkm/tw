# tw — tmux workspace manager

## What this is

A Go CLI tool that adds a visual workspace sidebar and notification system to tmux. Think of it as the cmux sidebar experience but for Ghostty + tmux (or any terminal + tmux). Agent-agnostic — works with Claude Code, Codex, Kiro, or any CLI tool.

## Current state

**Working:**
- `tw sidebar` — interactive TUI sidebar (bubbletea) showing all tmux sessions, windows, and notifications. Vim-style j/k navigation, Enter to switch sessions.
- `tw toggle` — toggles sidebar pane in tmux (bound to `prefix+b`)
- `tw notify <message>` — sends a notification for the current tmux session. Writes state to `~/.local/state/tmux-workspace/` and fires a macOS notification with sound.
- `tw clear [session]` / `tw clear --all` — clears notifications
- `tw status` — tmux status bar widget showing notification badge count
- Claude Code hooks in `~/.claude/settings.json` call `tw notify` on Stop, Notification, and SubagentStop events

**Architecture:**
```
main.go              — CLI entry (cobra), toggle logic
internal/tmux/       — tmux command wrapper (list sessions, windows, switch, etc.)
internal/notify/     — notification state (file-based in ~/.local/state/tmux-workspace/)
internal/ui/         — bubbletea TUI for the sidebar
bin/                 — old bash scripts (can be removed, superseded by Go binary)
install.sh           — old installer for bash scripts
```

## What's next

### Polish
- [ ] Sidebar visual refinement — better padding, colors, maybe session icons
- [ ] Fix: highlight should clearly follow cursor vs showing current session
- [ ] Smooth the toggle — sometimes detects sidebar pane incorrectly
- [ ] Add scrolling for many sessions (viewport component from bubbles)

### Features
- [ ] Session creation from sidebar (`n` to create new session)
- [ ] Session renaming from sidebar (`r`)
- [ ] Session kill from sidebar (`x` with confirmation)
- [ ] Window switching within the sidebar (not just sessions)
- [ ] Git branch display per session
- [ ] Configurable sidebar width
- [ ] Config file support (`~/.config/tw/config.toml`)

### Distribution
- [ ] Create GitHub repo (public)
- [ ] Set up goreleaser for cross-platform builds
- [ ] Create Homebrew tap (`homebrew-tap` repo) with formula
- [ ] Write a proper README with screenshots/GIFs

### Agent integration
- [ ] Document hook setup for each agent (Claude Code, Codex, Kiro, Aider)
- [ ] Consider a `tw init` command that auto-configures hooks
- [ ] Show which agent is running in each session (detect process)

## Key files outside this repo

- `~/.claude/settings.json` — Claude Code hooks pointing to `tw notify`
- `~/repos/dotfiles/tmux/tmux.conf` — tmux config with `tw toggle` and `tw status` bindings
- `~/.local/bin/tw` — installed binary location
- `~/.local/state/tmux-workspace/` — notification state files

## Dependencies

- Go 1.21+
- bubbletea, lipgloss, bubbles (charmbracelet)
- cobra (CLI framework)
- tmux (obviously)

## Build & install

```bash
cd ~/repos/tmux-workspace
go build -o tw .
cp tw ~/.local/bin/tw
```
