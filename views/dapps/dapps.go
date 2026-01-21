package dapps

import (
	"charm-wallet-tui/config"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for dApp browser view
func Nav(width int, dappMode string) string {
	var left string
	if dappMode == "add" || dappMode == "edit" {
		left = strings.Join([]string{
			styles.Key("l") + " debug log",
			styles.Key("Esc") + " cancel",
		}, "   ")
	} else {
		left = strings.Join([]string{
			styles.Key("↑/↓") + " select",
			styles.Key("Enter") + " open",
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

// Render renders the dApp browser view
func Render(dapps []config.DApp, selectedIdx int) string {
	h := styles.TitleStyle.Render("dApp Browser")

	lines := []string{h, ""}

	if len(dapps) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("No dApps configured."))
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Press ")+styles.Key("a")+lipgloss.NewStyle().Foreground(styles.CMuted).Render(" to add your first dApp."))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Available dApps:"))
		lines = append(lines, "")

		for i, dapp := range dapps {
			var marker string
			nameStyle := lipgloss.NewStyle().Foreground(styles.CText)
			addrStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

			if i == selectedIdx {
				nameStyle = nameStyle.Background(styles.CPanel).Foreground(styles.CAccent2).Bold(true)
				addrStyle = addrStyle.Background(styles.CPanel)
				marker = lipgloss.NewStyle().Foreground(styles.CAccent2).Render("▶ ")
			} else {
				marker = "  "
			}

			icon := dapp.Icon
			if icon == "" {
				icon = "🌐"
			}

			// Show network if set
			networkInfo := ""
			if dapp.Network != "" {
				networkInfo = lipgloss.NewStyle().Foreground(styles.CAccent).Render(" [" + dapp.Network + "]")
			}

			line := marker + icon + " " + nameStyle.Render(dapp.Name) + networkInfo
			lines = append(lines, line)
			lines = append(lines, "  "+addrStyle.Render(dapp.Address))
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}
