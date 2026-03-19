package uniswap

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
func Nav(width int, poolMonitorActive, liquidityActive bool) string {
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

	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("Tab") + " next",
		styles.Key("Shift+Tab") + " prev",
		styles.Key("m") + " max",
		styles.Key("Enter") + " select/swap",
		styles.Key("Esc") + " back",
		styles.Key("l") + " logger",
		pItem,
		qItem,
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
	tokenBoxStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder)
	
	tokenBoxFocusedStyle := tokenBoxStyle.Copy().
		BorderForeground(styles.CAccent).
		BorderStyle(lipgloss.ThickBorder())
	
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
	
	swapButtonFocusedStyle := swapButtonStyle.Copy().
		BorderForeground(styles.CAccent).
		BorderStyle(lipgloss.ThickBorder()).
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

		normalCard := lipgloss.NewStyle().
			Width(cardWidth).
			Padding(0, 2).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(styles.CBorder)

		focusedCard := lipgloss.NewStyle().
			Width(cardWidth).
			Padding(0, 2).
			BorderStyle(lipgloss.ThickBorder()).
			BorderForeground(styles.CAccent)

		labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		valueStyle := lipgloss.NewStyle().Foreground(styles.CAccent2)
		accentStyle := lipgloss.NewStyle().Foreground(styles.CAccent)
		boldStyle := lipgloss.NewStyle().Foreground(styles.CText).Bold(true)
		mutedStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

		var cards []string
		for i, pos := range positions {
			feePercent := fmt.Sprintf("%.2f%%", float64(pos.Fee)/10000.0)
			pair := pos.Token0Symbol + "/" + pos.Token1Symbol
			idStr := "#" + pos.TokenID.String()

			headerLine := boldStyle.Render(pair) +
				"   " + valueStyle.Render(feePercent) +
				"   " + mutedStyle.Render(idStr)

			minStr := liquidityFormatPrice(pos.MinPrice)
			maxStr := liquidityFormatPrice(pos.MaxPrice)
			rangeLine := labelStyle.Render("Range:  ") +
				valueStyle.Render(minStr+" — "+maxStr) +
				mutedStyle.Render("  "+pos.Token1Symbol+"/"+pos.Token0Symbol)

			liqLine := labelStyle.Render("Liq:    ") +
				lipgloss.NewStyle().Foreground(styles.CText).Render(pos.Liquidity.String())

			content := headerLine + "\n" + rangeLine + "\n" + liqLine

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
		Render("↑/↓ navigate   q/Esc back to swap")

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
