package uniswap

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/store"
	"charm-wallet-tui/styles"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/ethereum/go-ethereum/common"
)

// TokenOption represents a token available for swapping
type TokenOption struct {
	Symbol   string
	Balance  *big.Int
	Decimals uint8
	IsETH    bool
}

// Nav returns the navigation bar for Uniswap view.
// poolMonitorActive controls the color of the pool event monitor hotkey.
// liquidityActive controls the color of the liquidity hotkey.
// blockScanActive controls the color of the block scan hotkey.
func Nav(width int, poolMonitorActive, liquidityActive, indexerActive, blockScanActive bool) string {
	var pItem string
	if poolMonitorActive {
		pKey := lipgloss.NewStyle().Foreground(styles.CError).Bold(true).Render("p")
		pLabel := lipgloss.NewStyle().Foreground(styles.CWarn).Render("pool event monitor")
		pItem = pKey + " " + pLabel
	} else {
		pItem = styles.Key("p") + " pool event monitor"
	}

	var qItem string
	if liquidityActive {
		qKey := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("q")
		qLabel := lipgloss.NewStyle().Foreground(styles.CAccent2).Render("liquidity positions")
		qItem = qKey + " " + qLabel
	} else {
		qItem = styles.Key("q") + " liquidity positions"
	}

	var iItem string
	if indexerActive {
		iKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("i")
		iLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("indexer")
		iItem = iKey + " " + iLabel
	} else {
		iItem = styles.Key("i") + " indexer"
	}

	var bItem string
	if blockScanActive {
		bKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("b")
		bLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("block scan")
		bItem = bKey + " " + bLabel
	} else {
		bItem = styles.Key("b") + " block scan"
	}

	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("Tab") + " next",
		styles.Key("Shift+Tab") + " prev",
		styles.Key("m") + " max",
		styles.Key("Enter") + " select/swap",
		styles.Key("Esc") + " back",
		styles.Key("l") + " logger",
		iItem,
		pItem,
		qItem,
		bItem,
	}, "   ")

	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the Uniswap swap interface
func Render(width, height int, tokens []TokenOption, fromIdx, toIdx int, fromAmount, toAmount string, focusedField int, estimating bool, priceImpactWarn string) string {
	// Create the main swap container
	containerWidth := helpers.Min(80, width-4)
	
	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(containerWidth)
	
	title := titleStyle.Render("🦄 Uniswap Swap")
	
	// Token selection styles
	tokenBoxStyle := styles.CardNormal.Width(containerWidth - 4)
	tokenBoxFocusedStyle := styles.CardFocused.Width(containerWidth - 4)
	
	// Build "From" token section
	fromToken := ""
	fromBalance := "0"
	if fromIdx >= 0 && fromIdx < len(tokens) {
		fromToken = tokens[fromIdx].Symbol
		if tokens[fromIdx].Balance != nil {
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokens[fromIdx].Decimals)), nil))
			balance := new(big.Float).Quo(new(big.Float).SetInt(tokens[fromIdx].Balance), divisor)
			fromBalance = balance.Text('f', 6)
		}
	}
	
	fromLabel := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render("From")
	
	fromTokenDisplay := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Render(fromToken)
	
	fromBalanceDisplay := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render(fmt.Sprintf("Balance: %s", fromBalance))
	
	fromAmountDisplay := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Width(containerWidth - 8).
		Render(fromAmount)
	
	if fromAmount == "" {
		fromAmountDisplay = lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Width(containerWidth - 8).
			Render("0.0")
	}
	
	fromContent := fromLabel + "\n" +
		fromTokenDisplay + "   " + fromBalanceDisplay + "\n" +
		fromAmountDisplay
	
	var fromBox string
	if focusedField == 0 {
		fromBox = tokenBoxFocusedStyle.Render(fromContent)
	} else {
		fromBox = tokenBoxStyle.Render(fromContent)
	}
	
	// Swap arrow (centered)
	swapArrow := lipgloss.NewStyle().
		Foreground(styles.CAccent).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("⬇")
	
	// Build "To" token section
	toToken := ""
	toBalance := "0"
	if toIdx >= 0 && toIdx < len(tokens) {
		toToken = tokens[toIdx].Symbol
		if tokens[toIdx].Balance != nil {
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokens[toIdx].Decimals)), nil))
			balance := new(big.Float).Quo(new(big.Float).SetInt(tokens[toIdx].Balance), divisor)
			toBalance = balance.Text('f', 6)
		}
	}
	
	toLabel := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render("To")
	
	toTokenDisplay := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Render(toToken)
	
	toBalanceDisplay := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render(fmt.Sprintf("Balance: %s", toBalance))
	
	toAmountDisplay := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Width(containerWidth - 8).
		Render(toAmount)
	
	if toAmount == "" || estimating {
		displayText := "0.0"
		if estimating {
			displayText = "Estimating..."
		}
		toAmountDisplay = lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Width(containerWidth - 8).
			Render(displayText)
	}
	
	toContent := toLabel + "\n" +
		toTokenDisplay + "   " + toBalanceDisplay + "\n" +
		toAmountDisplay
	
	var toBox string
	if focusedField == 1 {
		toBox = tokenBoxFocusedStyle.Render(toContent)
	} else {
		toBox = tokenBoxStyle.Render(toContent)
	}
	
	// Price impact warning (if any)
	var warningDisplay string
	if priceImpactWarn != "" {
		warningStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF8C00")). // Orange color
			Width(containerWidth).
			Align(lipgloss.Center)
		warningDisplay = warningStyle.Render(priceImpactWarn)
	}
	
	// Swap button
	swapButtonStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 1).
		Align(lipgloss.Center).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder).
		Foreground(styles.CText)
	
	swapButtonFocusedStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 1).
		Align(lipgloss.Center).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(styles.CAccent).
		Background(styles.CAccent).
		Foreground(lipgloss.Color("#000000")).
		Bold(true)
	
	var swapButton string
	if focusedField == 2 {
		swapButton = swapButtonFocusedStyle.Render("Swap")
	} else {
		swapButton = swapButtonStyle.Render("Swap")
	}
	
	// Info text
	infoText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("↑/↓ navigate • Tab switch • Enter select")
	
	// Combine all elements with minimal spacing
	var contentParts []string
	contentParts = append(contentParts, title, "", fromBox, swapArrow, toBox)
	
	// Add warning if present
	if warningDisplay != "" {
		contentParts = append(contentParts, "", warningDisplay)
	}
	
	contentParts = append(contentParts, "", swapButton, "", infoText)
	
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		contentParts...,
	)
	
	// Center horizontally only, let panel handle vertical spacing
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)
}

// RenderTokenSelector renders a token selection popup
func RenderTokenSelector(width, height int, tokens []TokenOption, selectedIdx int, isForFromField bool) string {
	title := "Select Token"
	if isForFromField {
		title = "Select Token to Swap From"
	} else {
		title = "Select Token to Swap To"
	}
	
	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(60)
	
	// Token list
	var tokenList []string
	for i, token := range tokens {
		balance := "0"
		if token.Balance != nil {
			divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(token.Decimals)), nil))
			bal := new(big.Float).Quo(new(big.Float).SetInt(token.Balance), divisor)
			balance = bal.Text('f', 6)
		}
		
		marker := "  "
		style := lipgloss.NewStyle().Foreground(styles.CText)
		
		if i == selectedIdx {
			marker = "▸ "
			style = lipgloss.NewStyle().
				Foreground(styles.CAccent).
				Bold(true)
		}
		
		line := fmt.Sprintf("%s%s - Balance: %s", marker, token.Symbol, balance)
		tokenList = append(tokenList, style.Render(line))
	}
	
	listContent := strings.Join(tokenList, "\n")
	
	boxStyle := lipgloss.NewStyle().
		Width(60).
		Padding(2, 3).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CAccent).
		Background(styles.CPanel)
	
	content := titleStyle.Render(title) + "\n\n" + listContent
	
	box := boxStyle.Render(content)
	
	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

// RenderLiquidity renders the Uniswap V3 liquidity positions view.
func RenderLiquidity(width, height int, positions []helpers.LiquidityPosition, loading bool, focusedIdx int, errMsg, spinView string) string {
	containerWidth := helpers.Min(80, width-4)

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(containerWidth)

	title := titleStyle.Render("🦄 Liquidity Pools")

	var body string

	switch {
	case loading:
		body = lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Align(lipgloss.Center).
			Width(containerWidth).
			Render(spinView + " Loading positions…")

	case errMsg != "":
		body = lipgloss.NewStyle().
			Foreground(styles.CError).
			Align(lipgloss.Center).
			Width(containerWidth).
			Render("Error: " + errMsg)

	case len(positions) == 0:
		body = lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Align(lipgloss.Center).
			Width(containerWidth).
			Render("No V4 liquidity positions found for this address")

	default:
		cardWidth := containerWidth - 4
		normalCard := styles.CardNormal.Width(cardWidth)
		focusedCard := styles.CardFocused.Width(cardWidth)

		labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		valueStyle := lipgloss.NewStyle().Foreground(styles.CAccent2)
		accentStyle := lipgloss.NewStyle().Foreground(styles.CAccent)
		boldStyle := lipgloss.NewStyle().Foreground(styles.CText).Bold(true)
		mutedStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		warnStyle := lipgloss.NewStyle().Foreground(styles.CWarn)

		var cards []string
		for i, pos := range positions {
			idStr := "#" + pos.TokenID.String()

			// Stub card when positions() call failed entirely.
			if pos.Stub {
				content := boldStyle.Render(idStr) + "\n" +
					warnStyle.Render("positions() call failed — raw NFT token only")
				var card string
				if i == focusedIdx {
					card = focusedCard.Render(content)
				} else {
					card = normalCard.Render(content)
				}
				cards = append(cards, card)
				continue
			}

			feePercent := fmt.Sprintf("%.4f%%", float64(pos.Fee)/10000.0)
			pair := pos.Token0Symbol + "/" + pos.Token1Symbol

			headerLine := boldStyle.Render(pair) +
				"   " + valueStyle.Render(feePercent) +
				"   " + mutedStyle.Render(idStr)

			tok0Line := labelStyle.Render("Token0: ") + valueStyle.Render(pos.Token0Symbol) +
				mutedStyle.Render("  "+pos.Token0.Hex())
			tok1Line := labelStyle.Render("Token1: ") + valueStyle.Render(pos.Token1Symbol) +
				mutedStyle.Render("  "+pos.Token1.Hex())

			tickLine := labelStyle.Render("Ticks:  ") +
				valueStyle.Render(fmt.Sprintf("%d → %d", pos.TickLower, pos.TickUpper)) +
				mutedStyle.Render(fmt.Sprintf("  spacing=%d", pos.TickSpacing))

			minStr := liquidityFormatPrice(pos.MinPrice)
			maxStr := liquidityFormatPrice(pos.MaxPrice)
			rangeLine := labelStyle.Render("Range:  ") +
				valueStyle.Render(minStr+" — "+maxStr) +
				mutedStyle.Render("  "+pos.Token1Symbol+"/"+pos.Token0Symbol)

			liqVal := "nil"
			if pos.Liquidity != nil {
				liqVal = pos.Liquidity.String()
			}
			liqLine := labelStyle.Render("Liq:    ") +
				lipgloss.NewStyle().Foreground(styles.CText).Render(liqVal)

			hooksLine := labelStyle.Render("Hooks:  ") + mutedStyle.Render(pos.Hooks.Hex())

			content := headerLine + "\n" +
				tok0Line + "\n" +
				tok1Line + "\n" +
				tickLine + "\n" +
				rangeLine + "\n" +
				liqLine + "\n" +
				hooksLine

			if (pos.TokensOwed0 != nil && pos.TokensOwed0.Sign() > 0) ||
				(pos.TokensOwed1 != nil && pos.TokensOwed1.Sign() > 0) {
				f0 := liquidityFormatAmount(pos.TokensOwed0, pos.Token0Decimals)
				f1 := liquidityFormatAmount(pos.TokensOwed1, pos.Token1Decimals)
				feesLine := labelStyle.Render("Fees:   ") +
					accentStyle.Render(f0+" "+pos.Token0Symbol+" / "+f1+" "+pos.Token1Symbol)
				content += "\n" + feesLine
			}

			var card string
			if i == focusedIdx {
				card = focusedCard.Render(content)
			} else {
				card = normalCard.Render(content)
			}
			cards = append(cards, card)
		}
		body = strings.Join(cards, "\n")
	}

	infoText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("↑/↓ navigate   Esc back to swap")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title, "", body, "", infoText,
	)

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)
}

// liquidityFormatPrice formats a tick-derived price for display.
func liquidityFormatPrice(price float64) string {
	if price <= 0 || math.IsInf(price, 0) || math.IsNaN(price) {
		return "∞"
	}
	switch {
	case price < 0.0001:
		return fmt.Sprintf("%.8f", price)
	case price < 1:
		return fmt.Sprintf("%.6f", price)
	case price > 1e9:
		return fmt.Sprintf("%.2e", price)
	case price > 1e6:
		return fmt.Sprintf("%.2f", price)
	default:
		return fmt.Sprintf("%.4f", price)
	}
}

// v4scrollbarTrack builds a vertical scrollbar track (one char per visible line).
// Returns nil when there is nothing to scroll.
func v4scrollbarTrack(vpHeight, totalLines, yOffset int) []string {
	if totalLines <= vpHeight || vpHeight <= 0 {
		return nil
	}
	thumbSize := vpHeight * vpHeight / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxOffset := totalLines - vpHeight
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = (yOffset * (vpHeight - thumbSize)) / maxOffset
	}
	track := make([]string, vpHeight)
	for i := range track {
		if i >= thumbTop && i < thumbTop+thumbSize {
			track[i] = "█"
		} else {
			track[i] = "░"
		}
	}
	return track
}

// V4EventsContent builds the scrollable body string (pool cards) for the V4 Events panel.
// width is the outer panel width; the content is sized to fit inside it.
func V4EventsContent(width int, pools []store.PoolRow) string {
	containerWidth := helpers.Min(width-2, 120)

	if len(pools) == 0 {
		return lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Align(lipgloss.Center).
			Width(containerWidth).
			Render("Listening for V4 pool events…")
	}

	labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	accentStyle := lipgloss.NewStyle().Foreground(styles.CAccent)
	accent2Style := lipgloss.NewStyle().Foreground(styles.CAccent2)
	boldStyle := lipgloss.NewStyle().Foreground(styles.CText).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(styles.CWarn)
	cardWidth := containerWidth - 4
	card := styles.CardNormal.Width(cardWidth)

	var cards []string
	for _, r := range pools {
		poolLink := helpers.HyperPoolID(common.HexToHash(r.PoolID))

		tok0Sym := r.Token0Sym
		if tok0Sym == "" {
			tok0Sym = helpers.ShortenAddr(r.Currency0)
		}
		tok1Sym := r.Token1Sym
		if tok1Sym == "" {
			tok1Sym = helpers.ShortenAddr(r.Currency1)
		}

		pair := boldStyle.Render(tok0Sym) + labelStyle.Render(" / ") + accent2Style.Render(tok1Sym)
		feeStr := fmt.Sprintf("%.4f%%", float64(r.Fee)/10000.0)

		headerLine := pair +
			"   " + labelStyle.Render("fee:") + " " + accentStyle.Render(feeStr) +
			"   " + labelStyle.Render("swaps:") + " " + accentStyle.Render(fmt.Sprintf("%d", r.Swaps)) +
			"   " + labelStyle.Render("liq events:") + " " + accentStyle.Render(fmt.Sprintf("%d", r.LiqEvents))

		tok0Name := r.Token0Name
		if tok0Name == "" {
			tok0Name = labelStyle.Render("(unknown)")
		} else {
			tok0Name = labelStyle.Render(tok0Name)
		}
		tok1Name := r.Token1Name
		if tok1Name == "" {
			tok1Name = labelStyle.Render("(unknown)")
		} else {
			tok1Name = labelStyle.Render(tok1Name)
		}

		tok0Line := labelStyle.Render("Token0: ") +
			accentStyle.Render(tok0Sym) + "  " + tok0Name +
			"  " + labelStyle.Render(helpers.HyperAddr(common.HexToAddress(r.Currency0))) +
			"  " + labelStyle.Render("vol:") + " " + warnStyle.Render(v4FormatVolume(r.SwapVolume0))

		tok1Line := labelStyle.Render("Token1: ") +
			accent2Style.Render(tok1Sym) + "  " + tok1Name +
			"  " + labelStyle.Render(helpers.HyperAddr(common.HexToAddress(r.Currency1))) +
			"  " + labelStyle.Render("vol:") + " " + warnStyle.Render(v4FormatVolume(r.SwapVolume1))

		metaLine := labelStyle.Render("liq vol:") + " " + accentStyle.Render(v4FormatVolume(r.LiqVolume)) +
			"   " + labelStyle.Render("pool:") + " " + poolLink +
			"   " + labelStyle.Render("seen:") + " " + labelStyle.Render(r.SeenAt)

		content := headerLine + "\n" + tok0Line + "\n" + tok1Line + "\n" + metaLine
		cards = append(cards, card.Render(content))
	}
	return strings.Join(cards, "\n")
}

// RenderV4Events renders the V4 Events panel shown when the Pool Event Monitor is active.
// vp must have its content pre-set via V4EventsContent; width/height are the available dimensions.
func RenderV4Events(width, height int, vp viewport.Model) string {
	containerWidth := helpers.Min(width-2, 120)

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(containerWidth)
	title := titleStyle.Render("🦄 Uniswap V4 Events")

	infoText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("click pool ID → pool info   click address → Etherscan   ↑↓/PgUp/PgDn to scroll")

	// Reserve lines for title (1), blank (1), info (1), blank (1) = 4 lines overhead.
	vpHeight := helpers.Max(1, height-4)
	vp.Height = vpHeight

	vpContent := vp.View()
	track := v4scrollbarTrack(vpHeight, vp.TotalLineCount(), vp.YOffset)
	if len(track) > 0 {
		trackStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		vpLines := strings.Split(vpContent, "\n")
		for i := range vpLines {
			if i < len(track) {
				vpLines[i] = vpLines[i] + " " + trackStyle.Render(track[i])
			}
		}
		vpContent = strings.Join(vpLines, "\n")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", vpContent, "", infoText)
	return lipgloss.NewStyle().Width(width).Render(content)
}

// v4FormatVolume formats a raw token volume (float64) with K/M/B suffixes.
func v4FormatVolume(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v >= 1e12:
		return fmt.Sprintf("%.2fT", v/1e12)
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.2fK", v/1e3)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

// liquidityFormatAmount formats a token amount from base units to a human-readable string.
func liquidityFormatAmount(amount *big.Int, decimals uint8) string {
	if amount == nil || amount.Sign() == 0 {
		return "0"
	}
	divisor := new(big.Float).SetInt(
		new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil),
	)
	value := new(big.Float).Quo(new(big.Float).SetInt(amount), divisor)
	return value.Text('f', 6)
}
