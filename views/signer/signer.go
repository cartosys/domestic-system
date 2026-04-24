package signer

import (
	"fmt"
	"math/big"
	"strings"

	appsigner "charm-wallet-tui/signer"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation hint bar for the signer page.
func Nav(width int, indexerActive bool) string {
	items := []string{
		styles.Key("↑/↓") + " select key",
		styles.Key("s") + " scan QR",
		styles.Key("a") + " add key",
		styles.Key("c") + " clear",
		styles.Key("Esc") + " back",
	}
	if indexerActive {
		items = append([]string{styles.Key("i") + " indexer"}, items...)
	}
	return styles.NavStyle.Width(width).Render(strings.Join(items, "   "))
}

// Render produces the full signer page content string.
func Render(
	width int,
	keys []appsigner.KeyEntry,
	selectedIdx int,
	decoded *appsigner.DecodedTx,
	result *appsigner.SignResult,
	signErr string,
	scanning bool,
	spinView string,
) string {
	muted  := lipgloss.NewStyle().Foreground(styles.CMuted)
	accent := lipgloss.NewStyle().Foreground(styles.CAccent)
	accent2 := lipgloss.NewStyle().Foreground(styles.CAccent2)
	warn   := lipgloss.NewStyle().Foreground(styles.CWarn)
	errStyle := lipgloss.NewStyle().Foreground(styles.CError)

	halfW := helpers.Max(1, width/2-2)

	// ── Left panel: key list ──────────────────────────────────────────────────
	keyLines := []string{
		accent.Bold(true).Render("Stored Keys"),
		muted.Render(strings.Repeat("─", halfW)),
	}
	if len(keys) == 0 {
		keyLines = append(keyLines, muted.Render("No keys stored."))
		keyLines = append(keyLines, muted.Render("Press ")+styles.Key("a")+muted.Render(" to add one."))
	} else {
		for i, k := range keys {
			addr := helpers.ShortenAddr(k.Address)
			name := k.Name
			if i == selectedIdx {
				row := accent2.Bold(true).Render("▶ "+name) + "  " +
					lipgloss.NewStyle().Foreground(styles.CAccent).Render(addr)
				keyLines = append(keyLines, row)
			} else {
				row := muted.Render("  "+name) + "  " +
					muted.Render(addr)
				keyLines = append(keyLines, row)
			}
		}
	}

	// Pad key panel to a fixed height so columns align
	const keyPanelRows = 14
	for len(keyLines) < keyPanelRows {
		keyLines = append(keyLines, "")
	}
	leftPanel := strings.Join(keyLines, "\n")

	// ── Right panel: transaction / result ────────────────────────────────────
	rightLines := []string{
		accent.Bold(true).Render("Transaction"),
		muted.Render(strings.Repeat("─", halfW)),
	}

	switch {
	case scanning:
		rightLines = append(rightLines,
			spinView+" "+muted.Render("Scanning for EIP-4527 QR code…"),
		)

	case result != nil:
		keyFound := decoded != nil && decoded.From != "" &&
			len(keys) > 0 && selectedIdx >= 0 && selectedIdx < len(keys)
		fromDisplay := ""
		if decoded != nil {
			fromDisplay = helpers.ShortenAddr(decoded.From)
		}
		if keyFound {
			fromDisplay += " " + accent.Render("✓")
		}

		rightLines = append(rightLines,
			fmt.Sprintf("%s %s", muted.Render("From:  "), accent2.Render(fromDisplay)),
			fmt.Sprintf("%s %s", muted.Render("To:    "), lipgloss.NewStyle().Foreground(styles.CText).Render(helpers.ShortenAddr(result.To))),
			fmt.Sprintf("%s %s", muted.Render("Value: "), accent.Render(result.ValueHuman)),
			"",
			accent.Bold(true).Render("✓ Signed"),
			muted.Render("Hash: ")+lipgloss.NewStyle().Foreground(styles.CAccent2).Render(helpers.ShortenAddr(result.TxHash)),
			"",
			muted.Render("Raw tx:"),
			wrapHex(result.RawTx, halfW),
		)

	case signErr != "":
		rightLines = append(rightLines, errStyle.Render("Error: "+signErr))

	case decoded != nil:
		// Decoded but not yet signed (key mismatch)
		matchKey := appsigner.FindKey(decoded.From, keys)
		fromDisplay := helpers.ShortenAddr(decoded.From)
		var fromSuffix string
		if matchKey != "" {
			fromSuffix = " " + accent.Render("✓ key found")
		} else {
			fromSuffix = " " + warn.Render("✗ no key")
		}
		rightLines = append(rightLines,
			fmt.Sprintf("%s %s%s", muted.Render("From:  "), accent2.Render(fromDisplay), fromSuffix),
			fmt.Sprintf("%s %s", muted.Render("To:    "), lipgloss.NewStyle().Foreground(styles.CText).Render(helpers.ShortenAddr(decoded.To))),
			fmt.Sprintf("%s %s", muted.Render("Value: "), accent.Render(appsigner.WeiToEthStr(decoded.Value))),
			fmt.Sprintf("%s %d", muted.Render("Nonce: "), decoded.Nonce),
			fmt.Sprintf("%s %s gwei", muted.Render("Gas:   "), formatGwei(decoded.GasPrice)),
			fmt.Sprintf("%s %d", muted.Render("Limit: "), decoded.Gas),
			fmt.Sprintf("%s %s", muted.Render("Chain: "), decoded.ChainID.String()),
		)
		if matchKey != "" {
			rightLines = append(rightLines, "", muted.Render("Signing…"))
		}

	default:
		rightLines = append(rightLines,
			muted.Render("No transaction pending."),
			"",
			muted.Render("Press ")+styles.Key("s")+muted.Render(" to scan an EIP-4527 QR code."),
		)
	}

	rightPanel := strings.Join(rightLines, "\n")

	// ── Layout: two columns ───────────────────────────────────────────────────
	colStyle := lipgloss.NewStyle().Width(halfW).PaddingRight(2)
	row := lipgloss.JoinHorizontal(lipgloss.Top,
		colStyle.Render(leftPanel),
		colStyle.Render(rightPanel),
	)

	title := styles.TitleStyle.Render("Transaction Signer")
	return title + "\n\n" + row
}

// wrapHex wraps a long hex string at width characters for display.
func wrapHex(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	dim := lipgloss.NewStyle().Foreground(styles.CMuted)
	var b strings.Builder
	for i := 0; i < len(s); i += width {
		end := i + width
		if end > len(s) {
			end = len(s)
		}
		b.WriteString(dim.Render(s[i:end]))
		if end < len(s) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatGwei(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	gwei := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e9))
	return gwei.Text('f', 2)
}
