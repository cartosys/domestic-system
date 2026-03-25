package log

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// scrollbarTrack builds a slice of single-character strings (one per visible
// line) representing a vertical scrollbar track.  When there is nothing to
// scroll the slice is empty.
func scrollbarTrack(vpHeight, totalLines, yOffset int) []string {
	if totalLines <= vpHeight || vpHeight <= 0 {
		return nil
	}

	// Thumb height: at least 1, proportional to visible fraction.
	thumbSize := vpHeight * vpHeight / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}

	// Thumb top position within the track.
	maxOffset := totalLines - vpHeight
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = (yOffset * (vpHeight - thumbSize)) / maxOffset
	}

	track := make([]string, vpHeight)
	for i := range track {
		if i >= thumbTop && i < thumbTop+thumbSize {
			track[i] = "█"
		} else {
			track[i] = "░"
		}
	}
	return track
}

// Render renders the log panel.
// viewportHeight is the number of content lines the viewport should display;
// the caller is responsible for computing this from available screen space.
func Render(width, viewportHeight int, logReady bool, logSpinnerView string, vp viewport.Model, focused bool) string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Render("Log")

	logPanelHeight := helpers.Max(3, viewportHeight)

	// Update viewport height dynamically
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

	// Build the viewport body, optionally decorating with a scrollbar track.
	vpContent := vp.View()
	track := scrollbarTrack(logPanelHeight, vp.TotalLineCount(), vp.YOffset)
	if len(track) > 0 {
		trackStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		vpLines := strings.Split(vpContent, "\n")
		for i := range vpLines {
			if i < len(track) {
				vpLines[i] = vpLines[i] + " " + trackStyle.Render(track[i])
			}
		}
		vpContent = strings.Join(vpLines, "\n")
	}

	return border.Render(title + "\n\n" + vpContent)
}
