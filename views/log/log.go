package log

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// Render renders the log panel.
// viewportHeight is the number of content lines the viewport should display;
// the caller is responsible for computing this from available screen space.
func Render(width, viewportHeight int, logReady bool, logSpinnerView string, vp viewport.Model) string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Render("Log")

	logPanelHeight := helpers.Max(3, viewportHeight)

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
