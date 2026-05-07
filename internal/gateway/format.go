package gateway

import (
	"fmt"
	"strings"
)

func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func extractLastResponse(raw string) string {
	clean := stripAnsi(raw)
	lines := strings.Split(clean, "\n")

	// Trim trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	// Try to find the boundary where the agent started responding.
	// Look backwards for known idle prompts that indicate the agent finished.
	// The content between the last "sent message" and the idle prompt is the response.
	// As a fallback, return the last N non-empty lines.
	if len(lines) == 0 {
		return "(no output)"
	}

	// Simple heuristic: return up to 60 lines from the end, skipping trailing prompt
	start := 0
	if len(lines) > 60 {
		start = len(lines) - 60
	}

	result := strings.Join(lines[start:], "\n")
	result = strings.TrimSpace(result)
	if result == "" {
		return "(no output)"
	}
	return result
}

func formatOutput(session, raw string) string {
	response := extractLastResponse(raw)

	// Truncate for Slack message limit
	if len(response) > 3000 {
		response = response[:3000] + "\n... (truncated)"
	}

	return fmt.Sprintf("*%s*:\n```\n%s\n```", session, response)
}
