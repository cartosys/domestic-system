package txqr

import (
	"strings"

	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
)

var qrStyle = lipgloss.NewStyle().Foreground(styles.CAccent2)

// Render generates a terminal QR code from an EIP-4527 UR string for display in the TUI.
// All transaction QR codes in the app should be rendered through this function so that
// encoding stays consistent (RLP → CBOR → UR bytewords).
func Render(urString string) string {
	return qrStyle.Render(rpc.GenerateQRCode(urString))
}

// RenderAnimated splits urString into BCUR multi-part frames and returns compact
// half-block QR ASCII art for each frame.  maxChunkBytes controls frame count vs
// QR size; 50 bytes per chunk produces 3–6 frames for a typical ETH transaction
// and keeps each QR within ~60 terminal columns.
func RenderAnimated(urString string, maxChunkBytes int) ([]string, error) {
	frames, err := rpc.GenerateAnimatedQRFrames(urString, maxChunkBytes)
	if err != nil {
		return nil, err
	}

	// Split each frame into lines and find the maximum dimensions so all
	// frames can be padded to the same size before styling. Different BCUR
	// chunk sizes can produce different QR versions (module counts), which
	// shifts surrounding layout elements on every tick without this.
	split := make([][]string, len(frames))
	maxW, maxH := 0, 0
	for i, f := range frames {
		lines := strings.Split(f, "\n")
		split[i] = lines
		if len(lines) > maxH {
			maxH = len(lines)
		}
		for _, l := range lines {
			if w := lipgloss.Width(l); w > maxW {
				maxW = w
			}
		}
	}

	for i, lines := range split {
		for j, l := range lines {
			if pad := maxW - lipgloss.Width(l); pad > 0 {
				lines[j] = l + strings.Repeat(" ", pad)
			}
		}
		for len(lines) < maxH {
			lines = append(lines, strings.Repeat(" ", maxW))
		}
		frames[i] = qrStyle.Render(strings.Join(lines, "\n"))
	}
	return frames, nil
}
