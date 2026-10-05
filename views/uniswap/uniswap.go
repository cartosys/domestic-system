package uniswap

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/store"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/scrollbar"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum/common"
)

// TokenOption represents a token available for swapping
type TokenOption struct {
	Symbol   string
	Balance  *big.Int
	Decimals uint8
	IsETH    bool
	Address  common.Address
}

// Nav returns the navigation bar for Uniswap view.
// poolMonitorActive controls the color of the pool event monitor hotkey.
// liquidityActive controls the color of the liquidity hotkey.
// blockScanActive controls the color of the block scan hotkey.
func Nav(width int, poolMonitorActive, liquidityActive, blockScanActive, poolListActive bool) string {
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

	var bItem string
	if blockScanActive {
		bKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("b")
		bLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("block scan")
		bItem = bKey + " " + bLabel
	} else {
		bItem = styles.Key("b") + " block scan"
	}

	var oItem string
	if poolListActive {
		oKey := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("o")
		oLabel := lipgloss.NewStyle().Foreground(styles.CAccent2).Render("pool list")
		oItem = oKey + " " + oLabel
	} else {
		oItem = styles.Key("o") + " pool list"
	}

	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("m") + " max",
		styles.Key("l") + " logger",
		pItem,
		qItem,
		bItem,
		oItem,
	}, "   ")

	return styles.NavStyle.Width(width).Render(left)
}

// SwapGeometry reports hit-test rectangles for the From/To token boxes and
// the Swap button, relative to Render's own returned string (row/col 0 = its
// top-left corner). Measured from the same JoinVertical(Center, ...) plus
// outer Width/Align(Center) layout Render actually performs, mirroring the
// approach in terra.RenderClaimPopup's ClaimPopupGeometry.
type SwapGeometry struct {
	FromY, FromX1, FromX2, FromH int
	ToY, ToX1, ToX2, ToH         int
	SwapY, SwapX1, SwapX2, SwapH int
}

// Render renders the Uniswap swap interface
func Render(width, height int, tokens []TokenOption, fromIdx, toIdx int, fromAmount, toAmount string, focusedField int, estimating, resolvingPair bool, priceImpactWarn, hookWarn string) (string, SwapGeometry) {
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
	
	if toAmount == "" || estimating || resolvingPair {
		displayText := "0.0"
		if resolvingPair {
			displayText = "Finding pool..."
		} else if estimating {
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
			Foreground(styles.CWarn). // Orange color
			Width(containerWidth).
			Align(lipgloss.Center)
		warningDisplay = warningStyle.Render(priceImpactWarn)
	}

	// Hook-gated pool warning (if any) — separate banner since it's an
	// availability/compliance concern (KYC/geo allowlisting), not a pricing one.
	var hookWarningDisplay string
	if hookWarn != "" {
		hookWarningStyle := lipgloss.NewStyle().
			Foreground(styles.CWarn).
			Width(containerWidth).
			Align(lipgloss.Center)
		hookWarningDisplay = hookWarningStyle.Render(hookWarn)
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
		Foreground(styles.CBlack).
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
	
	// Combine all elements with minimal spacing, tracking each clickable
	// element's index in contentParts so geometry can be measured below.
	var contentParts []string
	contentParts = append(contentParts, title, "")
	fromBoxIdx := len(contentParts)
	contentParts = append(contentParts, fromBox, swapArrow)
	toBoxIdx := len(contentParts)
	contentParts = append(contentParts, toBox)

	// Add warnings if present
	if warningDisplay != "" {
		contentParts = append(contentParts, "", warningDisplay)
	}
	if hookWarningDisplay != "" {
		contentParts = append(contentParts, "", hookWarningDisplay)
	}

	contentParts = append(contentParts, "")
	swapButtonIdx := len(contentParts)
	contentParts = append(contentParts, swapButton, "", infoText)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		contentParts...,
	)

	// Center horizontally only, let panel handle vertical spacing
	rendered := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)

	// Replicate JoinVertical(Center, ...)'s own centering math (pad every
	// part to the widest part, then center each within that), plus the outer
	// Width/Align(Center) call's centering of that uniform-width block within
	// the full `width` — measured directly rather than re-derived from style
	// internals, so it stays correct if the styles change.
	maxW := 0
	for _, p := range contentParts {
		if w := lipgloss.Width(p); w > maxW {
			maxW = w
		}
	}
	rowY := func(idx int) int {
		y := 0
		for i := 0; i < idx; i++ {
			y += lipgloss.Height(contentParts[i])
		}
		return y
	}
	colX := func(s string) int { return (maxW - lipgloss.Width(s)) / 2 }
	outerPad := helpers.Max(0, (width-maxW)/2)

	geo := SwapGeometry{
		FromY: rowY(fromBoxIdx), FromX1: outerPad + colX(fromBox), FromH: lipgloss.Height(fromBox),
		ToY: rowY(toBoxIdx), ToX1: outerPad + colX(toBox), ToH: lipgloss.Height(toBox),
		SwapY: rowY(swapButtonIdx), SwapX1: outerPad + colX(swapButton), SwapH: lipgloss.Height(swapButton),
	}
	geo.FromX2 = geo.FromX1 + lipgloss.Width(fromBox)
	geo.ToX2 = geo.ToX1 + lipgloss.Width(toBox)
	geo.SwapX2 = geo.SwapX1 + lipgloss.Width(swapButton)

	return rendered, geo
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


// CardSpan records where one pool card sits in the content returned by PoolCards,
// in content-relative lines/columns, so mouse clicks can be mapped back to a pool.
// EndLine is exclusive. Close* is the ✕ hit zone (only set on the expanded card).
type CardSpan struct {
	PoolID             string
	StartLine, EndLine int
	Expanded           bool
	CloseLine          int
	CloseX1, CloseX2   int
}

// PoolDetailView is the live-state payload for the expanded pool card.
// MinHeight, when > 0, stretches the expanded card (border included) to at least
// that many lines, and Width, when > 0, sets its outer width (border included), so
// it fills the panel it is shown in.
type PoolDetailView struct {
	Loading   bool
	Err       string
	Data      *helpers.PoolDetails
	MinHeight int
	Width     int
}

// V4EventsContent builds the scrollable body string (pool cards) for the V4 Events panel.
// width is the outer panel width; the content is sized to fit inside it.
func V4EventsContent(width int, pools []store.PoolRow, expandedID string, detail PoolDetailView, cache map[string]string) (string, []CardSpan) {
	return PoolCards(width, pools, "Listening for V4 pool events…", expandedID, detail, cache)
}

// PoolCards renders one bordered card per pool. The card whose PoolID equals
// expandedID is drawn expanded with the live pool-state section and a ✕ close
// control; all others are the compact summary card. Shared by the V4 Events panel
// and the Pool List view so both render pools identically.
// cache, when non-nil, memoizes collapsed cards by pool ID; the caller must clear it
// when width changes or drop an entry when that pool's row data changes.
func PoolCards(width int, pools []store.PoolRow, emptyMsg, expandedID string, detail PoolDetailView, cache map[string]string) (string, []CardSpan) {
	containerWidth := helpers.Min(width-2, 120)

	if len(pools) == 0 {
		return lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Align(lipgloss.Center).
			Width(containerWidth).
			Render(emptyMsg), nil
	}

	cardWidth := containerWidth - 4
	card := styles.CardNormal.Width(cardWidth)
	expCardWidth := cardWidth
	if detail.Width > 0 {
		expCardWidth = helpers.Max(10, detail.Width-2)
	}
	expInnerWidth := expCardWidth - 4
	cardExpanded := styles.CardFocused.Width(expCardWidth)
	if detail.MinHeight > 2 {
		cardExpanded = cardExpanded.Height(detail.MinHeight - 2)
	}

	var cards []string
	var spans []CardSpan
	line := 0
	for _, r := range pools {
		var rendered string
		span := CardSpan{PoolID: r.PoolID}
		if r.PoolID == expandedID {
			closeStyle := lipgloss.NewStyle().Foreground(styles.CError).Bold(true)
			titleStyle := lipgloss.NewStyle().Foreground(styles.CBorder).Bold(true).Align(lipgloss.Center).Width(expInnerWidth - 2)
			topLine := titleStyle.Render("pool details") + " " + closeStyle.Render("✕")
			rendered = cardExpanded.Render(topLine + "\n" + poolCardBody(r, expCardWidth, false) + "\n" + poolDetailSection(r, detail, expInnerWidth))
			// Border(1) + padding(2) puts content column 0 at card column 3.
			closeCol := 3 + expInnerWidth - 1
			span.Expanded = true
			span.CloseLine = line + 1
			span.CloseX1 = closeCol - 1
			span.CloseX2 = closeCol + 2
		} else if c, ok := cache[r.PoolID]; ok {
			rendered = c
		} else {
			rendered = card.Render(poolCardBody(r, cardWidth, true))
			if cache != nil {
				cache[r.PoolID] = rendered
			}
		}
		h := lipgloss.Height(rendered)
		span.StartLine = line
		span.EndLine = line + h
		line += h
		cards = append(cards, rendered)
		spans = append(spans, span)
	}
	return strings.Join(cards, "\n"), spans
}

// poolCardBody renders the summary lines shared by collapsed and expanded cards.
// withType prepends the centered "initialize" event-type line used by the compact card.
func poolCardBody(r store.PoolRow, cardWidth int, withType bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	accentStyle := lipgloss.NewStyle().Foreground(styles.CAccent)
	accent2Style := lipgloss.NewStyle().Foreground(styles.CAccent2)
	boldStyle := lipgloss.NewStyle().Foreground(styles.CText).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(styles.CWarn)
	eventTypeStyle := lipgloss.NewStyle().Foreground(styles.CBorder).Bold(true).Align(lipgloss.Center)

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

	vol0 := r.SwapVolume0 / math.Pow(10, float64(r.Decimals0))
	vol1 := r.SwapVolume1 / math.Pow(10, float64(r.Decimals1))
	liqVol := r.LiqVolume / math.Pow(10, 18)

	tok0Line := labelStyle.Render("Token0: ") +
		accentStyle.Render(tok0Sym) + "  " + tok0Name +
		"  " + labelStyle.Render(helpers.HyperAddr(common.HexToAddress(r.Currency0))) +
		"  " + labelStyle.Render("vol:") + " " + warnStyle.Render(v4FormatVolume(vol0))

	tok1Line := labelStyle.Render("Token1: ") +
		accent2Style.Render(tok1Sym) + "  " + tok1Name +
		"  " + labelStyle.Render(helpers.HyperAddr(common.HexToAddress(r.Currency1))) +
		"  " + labelStyle.Render("vol:") + " " + warnStyle.Render(v4FormatVolume(vol1))

	metaLine := labelStyle.Render("liq vol:") + " " + accentStyle.Render(v4FormatVolume(liqVol)) +
		"   " + labelStyle.Render("pool:") + " " + poolLink +
		"   " + labelStyle.Render("seen:") + " " + labelStyle.Render(r.SeenAt)

	txHash := common.HexToHash(r.TxHash)
	txShort := txHash.Hex()[:10] + "…" + txHash.Hex()[len(txHash.Hex())-6:]
	txLink := ansi.SetHyperlink("https://etherscan.io/tx/"+txHash.Hex()) +
		helpers.FadeString(txShort, "#7D5AFC", "#FF87D7") +
		ansi.ResetHyperlink()
	blockLine := labelStyle.Render("block:") + " " + accentStyle.Render(fmt.Sprintf("%d", r.Block)) +
		"   " + labelStyle.Render("tx:") + " " + txLink

	var hooksLine string
	hooksAddr := common.HexToAddress(r.Hooks)
	if hooksAddr != (common.Address{}) {
		hooksLine = "\n" + labelStyle.Render("hooks:") + " " + helpers.HyperAddr(hooksAddr)
	}

	content := headerLine + "\n" + tok0Line + "\n" + tok1Line + "\n" + metaLine + "\n" + blockLine + hooksLine
	if withType {
		content = eventTypeStyle.Width(cardWidth).Render("initialize") + "\n" + content
	}
	return content
}

// poolDetailSection renders the live StateView read-out shown inside an expanded pool card.
func poolDetailSection(r store.PoolRow, detail PoolDetailView, innerWidth int) string {
	labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	accentStyle := lipgloss.NewStyle().Foreground(styles.CAccent)
	accent2Style := lipgloss.NewStyle().Foreground(styles.CAccent2)
	headStyle := lipgloss.NewStyle().Foreground(styles.CBorder).Bold(true)

	rule := func(title string) string {
		pad := helpers.Max(0, innerWidth-lipgloss.Width(title)-4)
		return headStyle.Render("── " + title + " " + strings.Repeat("─", pad))
	}
	kv := func(label, value string) string {
		return labelStyle.Render(label+":") + " " + accentStyle.Render(value)
	}
	big2s := func(x *big.Int) string {
		if x == nil {
			return "—"
		}
		return x.String()
	}

	switch {
	case detail.Loading:
		return rule("Pool State") + "\n" + labelStyle.Render("Loading live pool state from StateView…")
	case detail.Err != "":
		return rule("Pool State") + "\n" + lipgloss.NewStyle().Foreground(styles.CError).Width(innerWidth).Render("Error: "+detail.Err)
	case detail.Data == nil:
		return rule("Pool State") + "\n" + labelStyle.Render("No data")
	}
	d := detail.Data

	tok0, tok1 := r.Token0Sym, r.Token1Sym
	if tok0 == "" {
		tok0 = "token0"
	}
	if tok1 == "" {
		tok1 = "token1"
	}

	var lines []string
	lines = append(lines, rule("Pool State"))
	lines = append(lines, kv("sqrtPriceX96", big2s(d.SqrtPriceX96)))
	lines = append(lines, kv("price", liquidityFormatPrice(poolSqrtPriceToPrice(d.SqrtPriceX96, r.Decimals0, r.Decimals1)))+
		" "+labelStyle.Render(tok1+" per "+tok0))
	lines = append(lines, kv("current tick", fmt.Sprintf("%d", d.Tick))+"   "+kv("tick spacing", fmt.Sprintf("%d", d.TickSpacing)))
	lines = append(lines, kv("LP fee", fmt.Sprintf("%.4f%% (%d)", float64(d.LpFee)/10000.0, d.LpFee)))
	pf0, pf1 := d.ProtocolFee&0xfff, d.ProtocolFee>>12
	lines = append(lines, kv("protocol fee", fmt.Sprintf("0→1 %.4f%%  1→0 %.4f%% (%d)", float64(pf0)/10000.0, float64(pf1)/10000.0, d.ProtocolFee)))
	lines = append(lines, kv("active liquidity", big2s(d.Liquidity)))
	lines = append(lines, kv("fee growth global0", big2s(d.FeeGrowthGlobal0)))
	lines = append(lines, kv("fee growth global1", big2s(d.FeeGrowthGlobal1)))

	lines = append(lines, "", rule("Tick Bitmap"))
	if d.BitmapWord == nil {
		lines = append(lines, labelStyle.Render("unavailable (unknown tick spacing)"))
	} else {
		set := 0
		for i := 0; i < 256; i++ {
			set += int(d.BitmapWord.Bit(i))
		}
		lines = append(lines, kv("word position", fmt.Sprintf("%d", d.BitmapWordPos))+"   "+kv("initialized ticks in word", fmt.Sprintf("%d", set)))
		lines = append(lines, kv("word", fmt.Sprintf("0x%064x", d.BitmapWord)))
	}

	tickBlock := func(name string, t *helpers.TickDetails) []string {
		if t == nil {
			return []string{labelStyle.Render(name + ": no initialized tick within ±1 bitmap word")}
		}
		return []string{
			accent2Style.Render(fmt.Sprintf("%s tick %d", name, t.Tick)),
			"  " + kv("liquidityGross", big2s(t.LiquidityGross)) + "   " + kv("liquidityNet", big2s(t.LiquidityNet)),
			"  " + kv("feeGrowthOutside0", big2s(t.FeeGrowthOutside0)),
			"  " + kv("feeGrowthOutside1", big2s(t.FeeGrowthOutside1)),
		}
	}
	lines = append(lines, "", rule("Ticks Around Current Price"))
	lines = append(lines, tickBlock("lower", d.Lower)...)
	lines = append(lines, tickBlock("upper", d.Upper)...)

	lines = append(lines, "", rule("Fee Growth Inside Active Range"))
	if d.Lower != nil && d.Upper != nil {
		lines = append(lines, labelStyle.Render(fmt.Sprintf("range [%d, %d)", d.Lower.Tick, d.Upper.Tick)))
		lines = append(lines, kv("feeGrowthInside0", big2s(d.FeeGrowthInside0)))
		lines = append(lines, kv("feeGrowthInside1", big2s(d.FeeGrowthInside1)))
	} else {
		lines = append(lines, labelStyle.Render("unavailable — need initialized ticks on both sides of the current tick"))
	}
	return strings.Join(lines, "\n")
}

// poolSqrtPriceToPrice converts sqrtPriceX96 to a decimal-adjusted token1-per-token0 price.
func poolSqrtPriceToPrice(sqrtPriceX96 *big.Int, dec0, dec1 int64) float64 {
	if sqrtPriceX96 == nil || sqrtPriceX96.Sign() == 0 {
		return 0
	}
	q96 := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 96))
	ratio := new(big.Float).Quo(new(big.Float).SetInt(sqrtPriceX96), q96)
	p, _ := new(big.Float).Mul(ratio, ratio).Float64()
	return p * math.Pow(10, float64(dec0-dec1))
}

// V4EventsViewportHeight is the card viewport height RenderV4Events uses for a panel of
// the given height: title (1), blank (1), blank (1), info (1) = 4 lines overhead, or
// the whole panel when a pool detail is open.
func V4EventsViewportHeight(height int, detailOpen bool) int {
	if detailOpen {
		return helpers.Max(1, height)
	}
	return helpers.Max(1, height-4)
}

// V4EventsViewportTop is the line offset of RenderV4Events' viewport within its output.
func V4EventsViewportTop(detailOpen bool) int {
	if detailOpen {
		return 0
	}
	return 2
}

// renderDetailPanel renders only the scrollbar-decorated viewport at the full panel
// height: the pool detail view covers the panel's title/search/hint chrome.
func renderDetailPanel(width, height int, vp viewport.Model) string {
	vp.Height = helpers.Max(1, height)
	track := scrollbar.Track(vp.Height, vp.TotalLineCount(), vp.YOffset)
	return lipgloss.NewStyle().Width(width).Render(scrollbar.Decorate(vp.View(), track))
}

// RenderV4Events renders the V4 Events panel shown when the Pool Event Monitor is active.
// vp must have its content pre-set via V4EventsContent; width/height are the available dimensions.
// With detailOpen, only the viewport (holding the pool detail) is drawn.
func RenderV4Events(width, height int, vp viewport.Model, detailOpen bool) string {
	if detailOpen {
		return renderDetailPanel(width, height, vp)
	}
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

	vpHeight := V4EventsViewportHeight(height, false)
	vp.Height = vpHeight

	track := scrollbar.Track(vpHeight, vp.TotalLineCount(), vp.YOffset)
	vpContent := scrollbar.Decorate(vp.View(), track)

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", vpContent, "", infoText)
	return lipgloss.NewStyle().Width(width).Render(content)
}

// PoolListHeaderLines is the number of lines RenderPoolList draws above its viewport
// (title, blank, search box (3), count line, blank), used for mouse hit-testing.
const PoolListHeaderLines = 7

// PoolListViewportHeight is the card viewport height RenderPoolList uses for a panel of
// the given height: the header lines above plus a blank and the info line below, or
// the whole panel when a pool detail is open.
func PoolListViewportHeight(height int, detailOpen bool) int {
	if detailOpen {
		return helpers.Max(1, height)
	}
	return helpers.Max(1, height-PoolListHeaderLines-2)
}

// PoolListViewportTop is the line offset of RenderPoolList's viewport within its output.
func PoolListViewportTop(detailOpen bool) int {
	if detailOpen {
		return 0
	}
	return PoolListHeaderLines
}

// RenderPoolList renders the Pool List view: a ticker search box above a scrollable
// list of pool cards. vp must have its content pre-set via PoolCards.
// With detailOpen, only the viewport (holding the pool detail) is drawn.
func RenderPoolList(width, height int, searchView string, searchFocused bool, vp viewport.Model, shown, total int, detailOpen bool) string {
	if detailOpen {
		return renderDetailPanel(width, height, vp)
	}
	containerWidth := helpers.Min(width-2, 120)

	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(containerWidth).
		Render("🦄 Pool List")

	searchStyle := styles.CardNormal
	if searchFocused {
		searchStyle = styles.CardFocused
	}
	search := searchStyle.Width(containerWidth - 4).Render(searchView)

	count := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Render(fmt.Sprintf("%d of %d matching pools loaded", shown, total))

	infoText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("type to filter by ticker   click card → expand   click ✕ → collapse   tab or / → search   esc close")

	vpHeight := PoolListViewportHeight(height, false)
	vp.Height = vpHeight

	track := scrollbar.Track(vpHeight, vp.TotalLineCount(), vp.YOffset)
	vpContent := scrollbar.Decorate(vp.View(), track)

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", search, count, "", vpContent, "", infoText)
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
