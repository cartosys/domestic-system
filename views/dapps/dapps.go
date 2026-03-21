package dapps

import (
	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for dApp browser view
func Nav(width int, indexerActive bool) string {
	var iItem string
	if indexerActive {
		iKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("i")
		iLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("indexer")
		iItem = iKey + " " + iLabel
	} else {
		iItem = styles.Key("i") + " indexer"
	}

	left := strings.Join([]string{
		styles.Key("Tab") + " select next",
		styles.Key("Enter") + " open",
		styles.Key("l") + " logger",
		iItem,
		styles.Key("Esc") + " back",
	}, "   ")

	return styles.NavStyle.Width(width).Render(left)
}

// dAppCardStyle returns the style for a dApp card (unfocused)
func dAppCardStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Width(28).
		Height(6).
		Align(lipgloss.Center, lipgloss.Center).
		Background(styles.CPanel).
		Padding(1, 2).
		BorderStyle(lipgloss.HiddenBorder())
}

// dAppCardFocusedStyle returns the style for a focused dApp card
func dAppCardFocusedStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Width(28).
		Height(6).
		Align(lipgloss.Center, lipgloss.Center).
		Background(styles.CPanel).
		Padding(1, 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("69")) // Purple-blue from composable-views
}

// renderDAppCard renders a single dApp card
func renderDAppCard(dapp config.DApp, focused bool) string {
	icon := dapp.Icon
	if icon == "" {
		icon = "🌐"
	}

	// Shorten the address with fadeString
	shortenedAddr := helpers.ShortenAddr(dapp.Address)
	fadedAddr := helpers.FadeString(shortenedAddr, "#F25D94", "#EDFF82")

	// Name styling
	nameStyle := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Align(lipgloss.Center)

	// Network badge
	networkBadge := ""
	if dapp.Network != "" {
		networkBadge = lipgloss.NewStyle().
			Foreground(styles.CAccent).
			Render("[" + dapp.Network + "]")
	}

	// Build card content
	content := icon + "\n\n" +
		nameStyle.Render(dapp.Name) + "\n" +
		fadedAddr

	if networkBadge != "" {
		content += "\n" + networkBadge
	}

	// Apply appropriate style
	if focused {
		return dAppCardFocusedStyle().Render(content)
	}
	return dAppCardStyle().Render(content)
}

// Render renders the dApp browser view with grid layout
func Render(width int, dapps []config.DApp, selectedIdx int) string {
	h := styles.TitleStyle.Render("dApp Browser")

	if len(dapps) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Render("No dApps configured.")
		return h + "\n\n" + emptyMsg
	}

	// Build grid of dApp cards
	const columnsPerRow = 3
	const horizontalSpacing = "  "
	var rows []string

	for i := 0; i < len(dapps); i += columnsPerRow {
		var rowCards []string
		for j := 0; j < columnsPerRow && i+j < len(dapps); j++ {
			idx := i + j
			focused := (idx == selectedIdx)
			card := renderDAppCard(dapps[idx], focused)
			rowCards = append(rowCards, card)

			if j < columnsPerRow-1 && i+j+1 < len(dapps) {
				rowCards = append(rowCards, horizontalSpacing)
			}
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, rowCards...)
		rows = append(rows, row)
	}

	grid := strings.Join(rows, "\n")
	out := h + "\n\n" + grid

	// Description panel for the highlighted dapp
	if selectedIdx >= 0 && selectedIdx < len(dapps) {
		desc := dapps[selectedIdx].Description
		if desc != "" {
			descWidth := width - 4
			headStyle := lipgloss.NewStyle().
				Foreground(styles.CAccent2).
				Width(descWidth).
				Align(lipgloss.Center)
			bodyStyle := lipgloss.NewStyle().
				Foreground(styles.CMuted).
				Width(descWidth).
				Align(lipgloss.Center)

			paragraphs := strings.Split(desc, "\n\n")
			var descLines []string
			for i, p := range paragraphs {
				if i == 0 {
					descLines = append(descLines, headStyle.Render(p))
				} else {
					descLines = append(descLines, bodyStyle.Render(p))
				}
			}
			out += "\n\n" + strings.Join(descLines, "\n\n")
		}
	}

	return out
}
