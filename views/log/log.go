package log

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// Render renders the log panel with dynamic height calculation
func Render(width, height int, logReady bool, logSpinnerView string, vp viewport.Model) string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Render("Log")

	// Calculate available height for log panel
	// Account for: header (3 lines), nav (1 line), title + borders (4 lines), margins (2 lines)
	reservedHeight := 10
	availableHeight := helpers.Max(5, height-reservedHeight)

	// Limit max height to 1/3 of screen or 15 lines, whichever is smaller
	maxLogHeight := helpers.Min(height/3, 15)
	logPanelHeight := helpers.Min(availableHeight, maxLogHeight)

	// Update viewport height dynamically
	vp.Height = logPanelHeight

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder).
		Padding(0, 1).
		Width(helpers.Max(0, width-2)).
		Height(logPanelHeight + 2) // +2 for title and spacing

	if !logReady {
		initMsg := "initializing...\n" + logSpinnerView
		return border.Render(title + "\n\n" + initMsg)
	}

	// Show scrollbar info if content is larger than viewport
	scrollInfo := ""
	if vp.TotalLineCount() > 0 {
		scrollPercent := int(vp.ScrollPercent() * 100)
		if vp.TotalLineCount() > vp.Height {
			scrollInfo = lipgloss.NewStyle().
				Foreground(styles.CMuted).
				Render(fmt.Sprintf(" [%d%%]", scrollPercent))
		}
	}

	titleWithScroll := title + scrollInfo

	return border.Render(titleWithScroll + "\n\n" + vp.View())
}
