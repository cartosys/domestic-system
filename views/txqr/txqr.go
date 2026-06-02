package txqr

import (
	"charm-wallet-tui/rpc"

	"github.com/charmbracelet/lipgloss"
)

// Render generates a terminal QR code from an EIP-4527 UR string for display in the TUI.
// All transaction QR codes in the app should be rendered through this function so that
// encoding stays consistent (RLP → CBOR → UR bytewords).
func Render(urString string) string {
	return lipgloss.NewStyle().Render(rpc.GenerateQRCode(urString))
}

// RenderAnimated splits urString into BCUR multi-part frames and returns compact
// half-block QR ASCII art for each frame.  maxChunkBytes controls frame count vs
// QR size; 50 bytes per chunk produces 3–6 frames for a typical ETH transaction
// and keeps each QR within ~60 terminal columns.
func RenderAnimated(urString string, maxChunkBytes int) ([]string, error) {
	return rpc.GenerateAnimatedQRFrames(urString, maxChunkBytes)
}
