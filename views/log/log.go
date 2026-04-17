package log

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/scrollbar"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// Render renders the log panel.
// viewportHeight is the number of content lines the viewport should display;
// the caller is responsible for computing this from available screen space.
func Render(width, viewportHeight int, logReady bool, logSpinnerView string, vp viewport.Model, focused bool) string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Render("Log")

	logPanelHeight := helpers.Max(3, viewportHeight)

	vp.Height = logPanelHeight

	borderColor := styles.CBorder
	if focused {
		borderColor = styles.CAccent
	}
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(helpers.Max(0, width-2)).
		Height(logPanelHeight + 2) // +2 for title and spacing

	if !logReady {
		initMsg := "initializing...\n" + logSpinnerView
		return border.Render(title + "\n\n" + initMsg)
	}

	track := scrollbar.Track(logPanelHeight, vp.TotalLineCount(), vp.YOffset)
	vpContent := scrollbar.Decorate(vp.View(), track)

	return border.Render(title + "\n\n" + vpContent)
}
