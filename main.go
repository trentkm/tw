package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/trent/tmux-workspace/internal/notify"
	"github.com/trent/tmux-workspace/internal/tmux"
	"github.com/trent/tmux-workspace/internal/ui"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tw",
		Short: "tmux workspace manager",
		Long:  "A workspace manager for tmux with notifications and a visual sidebar.",
	}

	// tw sidebar
	sidebarCmd := &cobra.Command{
		Use:   "sidebar",
		Short: "Open the interactive workspace sidebar",
		RunE: func(cmd *cobra.Command, args []string) error {
			m := ui.NewModel()
			p := tea.NewProgram(m, tea.WithAltScreen())
			_, err := p.Run()
			return err
		},
	}

	// tw notify
	notifyCmd := &cobra.Command{
		Use:   "notify [message]",
		Short: "Send a notification for the current tmux session",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session, _ := cmd.Flags().GetString("session")
			if session == "" {
				session = tmux.CurrentSession()
				if session == "" {
					session = "default"
				}
			}
			message := strings.Join(args, " ")
			return notify.Add(session, message)
		},
	}
	notifyCmd.Flags().StringP("session", "s", "", "Session name (default: current tmux session)")

	// tw clear
	clearCmd := &cobra.Command{
		Use:   "clear [session]",
		Short: "Clear notifications",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			if all {
				return notify.ClearAll()
			}
			if len(args) > 0 {
				return notify.Clear(args[0])
			}
			session := tmux.CurrentSession()
			if session != "" {
				return notify.Clear(session)
			}
			return fmt.Errorf("specify a session name or use --all")
		},
	}
	clearCmd.Flags().BoolP("all", "a", false, "Clear all notifications")

	// tw status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Status bar widget (for tmux status-right)",
		Run: func(cmd *cobra.Command, args []string) {
			count := notify.Count()
			if count > 0 {
				fmt.Printf("#[fg=yellow,bold] ● %d waiting #[default]", count)
			}
		},
	}

	// tw toggle
	toggleCmd := &cobra.Command{
		Use:   "toggle",
		Short: "Toggle the workspace sidebar in tmux",
		RunE: func(cmd *cobra.Command, args []string) error {
			width, _ := cmd.Flags().GetInt("width")
			return toggleSidebar(width)
		},
	}
	toggleCmd.Flags().IntP("width", "w", 30, "Sidebar width")

	rootCmd.AddCommand(sidebarCmd, notifyCmd, clearCmd, statusCmd, toggleCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func toggleSidebar(width int) error {
	// Check if sidebar pane exists
	out, err := tmux.Run("list-panes", "-F", "#{pane_id}|#{pane_pid}")
	if err != nil {
		return err
	}

	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		paneID, panePID := parts[0], parts[1]

		// Check if this pane is running tw sidebar
		checkCmd := exec.Command("pgrep", "-P", panePID, "-f", "tw sidebar")
		if checkCmd.Run() == nil {
			return tmux.KillPane(paneID)
		}
	}

	// No sidebar found — create one
	return tmux.SplitWindow("-hb", "-l", fmt.Sprintf("%d", width), "tw", "sidebar")
}
