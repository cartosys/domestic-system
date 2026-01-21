package settings

import (
	"charm-wallet-tui/config"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for settings view
func Nav(width int, settingsMode string) string {
	var left string
	if settingsMode == "add" || settingsMode == "edit" {
		left = strings.Join([]string{
			styles.Key("l") + " debug log",
			styles.Key("Esc") + " cancel",
		}, "   ")
	} else {
		left = strings.Join([]string{
			styles.Key("↑/↓") + " select",
			styles.Key("Enter") + " activate",
			styles.Key("a") + " add",
			styles.Key("e") + " edit",
			styles.Key("d") + " delete",
			styles.Key("h") + " home",
			styles.Key("l") + " debug log",
			styles.Key("Esc") + " back",
		}, "   ")
	}

	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the RPC settings view
func Render(rpcURLs []config.RPCUrl, selectedIdx int) string {
	h := styles.TitleStyle.Render("RPC Settings")

	// List mode
	lines := []string{h, ""}

	if len(rpcURLs) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("No RPC URLs configured."))
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Press ")+styles.Key("a")+lipgloss.NewStyle().Foreground(styles.CMuted).Render(" to add your first RPC URL."))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Configured RPC Endpoints:"))
		lines = append(lines, "")

		for i, rpc := range rpcURLs {
			var marker string
			if rpc.Active {
				marker = lipgloss.NewStyle().Foreground(styles.CAccent).Render("● ")
			} else {
				marker = lipgloss.NewStyle().Foreground(styles.CMuted).Render("○ ")
			}

			nameStyle := lipgloss.NewStyle().Foreground(styles.CText)
			urlStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

			if i == selectedIdx {
				nameStyle = nameStyle.Background(styles.CPanel).Foreground(styles.CAccent2).Bold(true)
				urlStyle = urlStyle.Background(styles.CPanel)
				marker = lipgloss.NewStyle().Foreground(styles.CAccent2).Render("▶ ")
			}

			line := marker + nameStyle.Render(rpc.Name)
			lines = append(lines, line)
			lines = append(lines, "  "+urlStyle.Render(rpc.URL))
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}
