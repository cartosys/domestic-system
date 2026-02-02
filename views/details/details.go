package details

import (
	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for details view
func Nav(width int, nicknaming bool) string {
	var left string
	if nicknaming {
		left = strings.Join([]string{
			styles.Key("l") + " logger",
			styles.Key("Esc") + " cancel",
		}, "   ")
	} else {
		left = strings.Join([]string{
			styles.Key("c") + " copy address",
			styles.Key("n") + " nickname",
			styles.Key("r") + " refresh",
			styles.Key("w") + " wallets",
			styles.Key("s") + " settings",
			styles.Key("b") + " dApps",
			styles.Key("l") + " logger",
			styles.Key("Esc") + " back",
		}, "   ")
	}

	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the account details view
func Render(details rpc.WalletDetails, wallets []config.WalletEntry, loading bool, copiedMsg string, spinnerView string) string {
	h := styles.TitleStyle.Render("Account Details")

	// Find nickname for current wallet
	var nickname string
	for _, w := range wallets {
		if strings.EqualFold(w.Address, details.Address) {
			nickname = w.Name
			break
		}
	}

	// Make address clickable with underline hint and hyperlink to Etherscan
	etherscanURL := fmt.Sprintf("https://etherscan.io/address/%s", details.Address)
	addrStyle := lipgloss.NewStyle().Foreground(styles.CMuted).Underline(true)
	// Use OSC 8 hyperlink format: \x1b]8;;URL\x1b\\TEXT\x1b]8;;\x1b\\
	sub := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", etherscanURL, addrStyle.Render(details.Address))

	// Add nickname if it exists
	if nickname != "" {
		nicknameStyle := lipgloss.NewStyle().Foreground(styles.CAccent2).Italic(true)
		sub = nicknameStyle.Render("\""+nickname+"\"") + "  " + sub
	}

	if copiedMsg != "" {
		sub += "  " + lipgloss.NewStyle().Foreground(styles.CAccent).Render(copiedMsg)
	}

	if loading {
		return h + "\n" + sub + "\n\n" + spinnerView + " fetching balances…"
	}

	if details.ErrMessage != "" {
		msg := lipgloss.NewStyle().Foreground(styles.CWarn).Render("⚠ " + details.ErrMessage)
		hint := lipgloss.NewStyle().Foreground(styles.CMuted).Render("Tip: set ") + lipgloss.NewStyle().Foreground(styles.CAccent).Render("ETH_RPC_URL") +
			lipgloss.NewStyle().Foreground(styles.CMuted).Render(" then press ") + styles.Key("r") + lipgloss.NewStyle().Foreground(styles.CMuted).Render(" to refresh.")
		return h + "\n" + sub + "\n\n" + msg + "\n\n" + hint
	}

	ethLine := fmt.Sprintf("%s  %s",
		lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("ETH"),
		lipgloss.NewStyle().Foreground(styles.CText).Render(helpers.FormatETH(details.EthWei)),
	)

	lines := []string{h, sub, "", ethLine, ""}

	if len(details.Tokens) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("No watched token balances found (non-zero)."))
		lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Edit tokenWatch in code (or add config) to track more tokens."))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, lipgloss.NewStyle().Foreground(styles.CMuted).Render("Tokens (watchlist)"))

	// table-ish rendering
	for _, t := range details.Tokens {
		row := fmt.Sprintf("%-6s  %s",
			lipgloss.NewStyle().Foreground(styles.CAccent).Render(t.Symbol),
			lipgloss.NewStyle().Foreground(styles.CText).Render(helpers.FormatToken(t.Balance, t.Decimals, t.Symbol)),
		)
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}
