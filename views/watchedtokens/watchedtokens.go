package watchedtokens

import (
	"strings"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
)

// HeaderLines is the number of lines Render emits before the first token row
// (title, blank, intro, blank) when tokens is non-empty.
const HeaderLines = 4

// RowHeight is the number of lines Render emits per token (symbol/balance
// line, address line, blank separator).
const RowHeight = 3

// Nav returns the navigation bar for the Watched Tokens view.
func Nav(width int, mode string, indexerActive bool) string {
	var iItem string
	if indexerActive {
		iKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("i")
		iLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("indexer")
		iItem = iKey + " " + iLabel
	} else {
		iItem = styles.Key("i") + " indexer"
	}

	var left string
	if mode == "add" || mode == "edit" {
		left = strings.Join([]string{
			styles.Key("l") + " logger",
			iItem,
			styles.Key("Esc") + " cancel",
		}, "   ")
	} else {
		left = strings.Join([]string{
			styles.Key("↑/↓") + " select",
			styles.Key("a") + " add",
			styles.Key("e") + " edit",
			styles.Key("del") + " delete",
			styles.Key("l") + " logger",
			iItem,
			styles.Key("Esc") + " back",
		}, "   ")
	}

	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the Watched Tokens list. tokens must already be sorted in
// the order they should display (highest active-wallet balance first).
// details supplies the active wallet's loaded token balances, matched back
// to each watched token by contract address.
func Render(tokens []rpc.WatchedToken, details rpc.WalletDetails, selectedIdx int) string {
	h := styles.TitleStyle.Render("Watched Tokens")

	lines := []string{h, ""}

	if len(tokens) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("No tokens watched."))
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Press ")+styles.Key("a")+lipgloss.NewStyle().Foreground(styles.CMuted).Render(" to add a token."))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Ordered by the active wallet's balance, highest first:"))
		lines = append(lines, "")

		for i, t := range tokens {
			nameStyle := lipgloss.NewStyle().Foreground(styles.CText)
			addrStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
			marker := "  "

			if i == selectedIdx {
				nameStyle = nameStyle.Background(styles.CPanel).Foreground(styles.CAccent2).Bold(true)
				addrStyle = addrStyle.Background(styles.CPanel)
				marker = lipgloss.NewStyle().Foreground(styles.CAccent2).Render("▶ ")
			}

			balanceText := "—"
			for _, tb := range details.Tokens {
				if tb.Address == t.Address && tb.Balance != nil {
					balanceText = helpers.FormatToken(tb.Balance, t.Decimals, t.Symbol)
					break
				}
			}

			label := t.Symbol
			if t.Name != "" {
				label = t.Symbol + " - " + t.Name
			}

			line := marker + nameStyle.Render(label) + "  " + addrStyle.Render(balanceText)
			lines = append(lines, line)
			lines = append(lines, "  "+addrStyle.Render(helpers.ShortenAddr(t.Address.Hex())))
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}
