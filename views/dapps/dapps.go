package dapps

import (
	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"math/big"
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

// renderDAppCard renders a single dApp card. chainID is the currently
// connected network, used as the card's network label when dapp.Network is
// blank (i.e. the dapp follows whichever network is active, rather than
// being pinned to one — see config.DefaultDapps).
func renderDAppCard(dapp config.DApp, focused bool, chainID *big.Int) string {
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

	// Network badge: dapp.Network pins a fixed label (e.g. Terra Nullius,
	// whose contract only ever exists on mainnet); blank means "follow the
	// active connection" (e.g. Uniswap v4, supported on both networks).
	label := dapp.Network
	if label == "" {
		label = helpers.ChainName(chainID)
	}
	networkBadge := lipgloss.NewStyle().
		Foreground(styles.CAccent).
		Render("[" + label + "]")

	// Build card content
	content := icon + "\n\n" +
		nameStyle.Render(dapp.Name) + "\n" +
		fadedAddr + "\n" +
		networkBadge

	// Apply appropriate style
	if focused {
		return dAppCardFocusedStyle().Render(content)
	}
	return dAppCardStyle().Render(content)
}

// Render renders the dApp browser view with grid layout. chainID is the
// currently connected network (see renderDAppCard).
func Render(width int, dapps []config.DApp, selectedIdx int, chainID *big.Int) string {
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
			card := renderDAppCard(dapps[idx], focused, chainID)
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
