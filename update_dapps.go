package main

import (
	"charm-wallet-tui/config"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleDappsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.activePage = config.PageWallets
		return m, m.loadSelectedWalletDetails()

	case "enter":
		if m.selectedDappIdx >= 0 && m.selectedDappIdx < len(m.dapps) &&
			m.dapps[m.selectedDappIdx].Name == "Terra Nullius" {
			m.activePage = config.PageTerraNullius
			m.terraNullFocusedField = 1
			m.terraNullClaimsCount = ""
			m.terraNullClaimsLoading = true
			m.terraNullClaimInput = "0"
			m.terraNullClaimResult = nil
			m.terraNullClaimQuerying = false
			m.terraNullClaimResultErr = ""
			m.activeDialog = dialogNone
			m.addLog("info", "Terra Nullius: loading number of claims…")
			return m, fetchTerraNumberOfClaims(m.ethClient)
		}
		// Default: open Uniswap interface
		m.activePage = config.PageUniswap
		m.uniswapFromTokenIdx = 0
		m.uniswapToTokenIdx = 1
		m.uniswapFromAmount = ""
		m.uniswapToAmount = ""
		m.uniswapFocusedField = 0
		m.uniswapShowingSelector = false
		m.uniswapSelectorFor = 0
		m.uniswapSelectorIdx = 0
		m.uniswapEstimating = false
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		m.lastQuoteFromAmount = ""
		m.lastQuoteFromTokenIdx = -1
		m.lastQuoteToTokenIdx = -1
		return m, nil

	case "tab", "down", "right":
		if len(m.dapps) > 0 {
			m.selectedDappIdx = (m.selectedDappIdx + 1) % len(m.dapps)
		}
		return m, nil

	case "shift+tab", "up", "left":
		if len(m.dapps) > 0 {
			m.selectedDappIdx--
			if m.selectedDappIdx < 0 {
				m.selectedDappIdx = len(m.dapps) - 1
			}
		}
		return m, nil
	}
	return m, nil
}
