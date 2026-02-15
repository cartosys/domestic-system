package uniswap

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"fmt"
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

// Nav returns the navigation bar for Uniswap view
func Nav(width int) string {
	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("Tab") + " next",
		styles.Key("Shift+Tab") + " prev",
		styles.Key("m") + " max",
		styles.Key("Enter") + " select/swap",
		styles.Key("Esc") + " back",
		styles.Key("l") + " logger",
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
