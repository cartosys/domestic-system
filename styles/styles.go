package styles

import "github.com/charmbracelet/lipgloss"

// Theme colors
var (
	CBg      = lipgloss.Color("#0B0F14") // near-black
	CPanel   = lipgloss.Color("#0F1720") // slightly lighter
	CBorder  = lipgloss.Color("#874BFD")
	CMuted   = lipgloss.Color("#8AA0B6")
	CText    = lipgloss.Color("#D6E2F0")
	CAccent  = lipgloss.Color("#7EE787") // green-ish
	CAccent2 = lipgloss.Color("#79C0FF") // blue-ish
	CWarn    = lipgloss.Color("#FFA657") // orange
)

// Shared styles
var (
	AppStyle = lipgloss.NewStyle().
			Background(CBg).
			Foreground(CText)

	TitleStyle = lipgloss.NewStyle().
			Foreground(CAccent2).
			Bold(true)

	PanelStyle = lipgloss.NewStyle().
			Background(CPanel).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(CBorder).
			Padding(1, 2)

	NavStyle = lipgloss.NewStyle().
			Background(CPanel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(CBorder).
			Padding(0, 1)

	HotkeyStyle = lipgloss.NewStyle().
			Foreground(CMuted)

	HotkeyKeyStyle = lipgloss.NewStyle().
			Foreground(CAccent).
			Bold(true)

	HelpRightStyle = lipgloss.NewStyle().
			Foreground(CMuted)
)

// Key renders a key with accent styling
func Key(s string) string {
	return HotkeyKeyStyle.Render(s)
}
