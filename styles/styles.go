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
	CError   = lipgloss.Color("#F25D94") // pink/red
	CInfo    = lipgloss.Color("#FFF7DB") // light yellow
	CGray    = lipgloss.Color("#888B7E") // neutral inactive button background
	CFail    = lipgloss.Color("#c01c28") // hard red for critical failures (RPC down etc.)
	CSuccess = lipgloss.Color("#00FF00") // bright green for copy/success confirmations
	CSubtle  = lipgloss.Color("#666666") // secondary hint text
	CBlack   = lipgloss.Color("#000000") // pure black for high-contrast button text
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

	// Component base styles — use .Copy() before adding per-call overrides
	DialogBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(CBorder).
			Padding(1, 2)

	CardNormal = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(CBorder).
			Padding(0, 2)

	CardFocused = lipgloss.NewStyle().
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(CAccent).
			Padding(0, 2)

	ButtonNormal = lipgloss.NewStyle().
			Foreground(CInfo).
			Background(CGray).
			Padding(0, 3)

	ButtonActive = lipgloss.NewStyle().
			Foreground(CInfo).
			Background(CError).
			Padding(0, 3).
			Underline(true)

	ButtonPrimary = lipgloss.NewStyle().
			Foreground(CInfo).
			Background(CBorder).
			Padding(0, 3).
			Underline(true)
)

// Key renders a key with accent styling
func Key(s string) string {
	return HotkeyKeyStyle.Render(s)
}
