package scrollbar

import (
	"strings"

	"charm-wallet-tui/styles"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// State holds per-viewport scrollbar interaction state.
// PanelTop and TrackCol are set by the render path each frame.
type State struct {
	Dragging bool
	PanelTop int // screen Y of first viewport content line
	TrackCol int // screen X of scrollbar characters
}

// Track builds the scrollbar character slice (one entry per visible line).
// Returns nil when content fits without scrolling.
func Track(vpHeight, totalLines, yOffset int) []string {
	if totalLines <= vpHeight || vpHeight <= 0 {
		return nil
	}
	thumbSize := vpHeight * vpHeight / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
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

// Decorate appends the scrollbar track to the right of each line of vpContent.
func Decorate(vpContent string, track []string) string {
	if len(track) == 0 {
		return vpContent
	}
	trackStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	lines := strings.Split(vpContent, "\n")
	for i := range lines {
		if i < len(track) {
			lines[i] = lines[i] + " " + trackStyle.Render(track[i])
		}
	}
	return strings.Join(lines, "\n")
}

// HitTest reports whether (mx, my) is on or within 1 column of the scrollbar track.
func (s State) HitTest(mx, my, vpBottom int) bool {
	return mx >= s.TrackCol-1 && mx <= s.TrackCol+1 &&
		my >= s.PanelTop && my <= vpBottom
}

// ApplyDrag maps a screen Y coordinate to a viewport scroll offset.
func (s State) ApplyDrag(screenY int, vp *viewport.Model) {
	vpHeight := vp.Height
	totalLines := vp.TotalLineCount()
	if vpHeight <= 0 || totalLines <= vpHeight {
		return
	}
	trackY := screenY - s.PanelTop
	maxOffset := totalLines - vpHeight
	newOffset := trackY * maxOffset / (vpHeight - 1)
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxOffset {
		newOffset = maxOffset
	}
	vp.YOffset = newOffset
}
