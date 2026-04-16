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
