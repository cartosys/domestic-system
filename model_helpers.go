package main

import (
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

// buildTokenWatchlist returns the starter token watchlist (WETH/USDC/USDT/DAI)
// pointed at the given network's token addresses, so balances are queried
// against the contracts that actually exist on the connected chain.
func buildTokenWatchlist(addrs helpers.UniswapNetworkAddresses) []rpc.WatchedToken {
	tokens := []rpc.WatchedToken{
		{Symbol: "WETH", Decimals: 18, Address: addrs.WETH},
		{Symbol: "USDC", Decimals: 6, Address: addrs.USDC},
		{Symbol: "USDT", Decimals: 6, Address: addrs.USDT},
		{Symbol: "DAI", Decimals: 18, Address: addrs.DAI},
	}
	if addrs.SPCXon != (common.Address{}) {
		tokens = append(tokens, rpc.WatchedToken{Symbol: "SPCXon", Decimals: 18, Address: addrs.SPCXon})
	}
	return tokens
}

// isDoubleClick returns true when (x, y) matches the previous click within 500 ms.
// Always updates last-click state so a single call both detects and advances the window.
func (m *model) isDoubleClick(x, y int) bool {
	const doubleClickWindow = 500 * time.Millisecond
	now := time.Now()
	isDouble := now.Sub(m.lastClickTime) < doubleClickWindow &&
		m.lastClickX == x && m.lastClickY == y
	m.lastClickTime = now
	m.lastClickX = x
	m.lastClickY = y
	return isDouble
}

// textInputActive returns true if any text input is currently capturing keystrokes.
func (m model) textInputActive() bool {
	return m.activeDialog == dialogAddWallet ||
		m.activeDialog == dialogEditWallet ||
		(m.activeDialog == dialogSendTx && m.sendForm != nil) ||
		(m.nicknaming && m.form != nil) ||
		((m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil) ||
		(m.activeDialog == dialogPasteSignedTx && m.pasteTxPhase == pasteTxPhaseForm && m.pasteTxForm != nil) ||
		m.activeDialog == dialogTerraClaim
}

// loadSelectedWalletDetails loads details for the currently selected wallet,
// serving from the cache when available.
func (m *model) loadSelectedWalletDetails() tea.Cmd {
	if !m.detailsInWallets || len(m.accounts) == 0 {
		return nil
	}
	addr := m.accounts[m.selectedWallet].Address
	if cached, ok := m.detailsCache[strings.ToLower(addr)]; ok {
		m.details = cached
		m.loading = false
		return nil
	}
	m.loading = true
	m.details = rpc.WalletDetails{Address: addr}
	return loadDetails(m.ethClient, common.HexToAddress(addr), m.tokenWatch)
}

// navigateTo sets the active page and issues any initial Cmds required for that page.
func (m *model) navigateTo(page config.Page) tea.Cmd {
	m.activePage = page
	switch page {
	case config.PageWallets:
		return m.loadSelectedWalletDetails()
	case config.PageSettings:
		m.settingsMode = "list"
	case config.PageUniswap:
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
	case config.PageTerraNullius:
		m.terraNullFocusedField = 1
		m.terraNullClaimsCount = ""
		m.terraNullClaimsLoading = true
		m.terraNullClaimInput = "0"
		m.terraNullClaimResult = nil
		m.terraNullClaimQuerying = false
		m.terraNullClaimResultErr = ""
		m.activeDialog = dialogNone
		m.logInfo("Terra Nullius: loading number of claims…")
		return fetchTerraNumberOfClaims(m.ethClient)
	}
	return nil
}

// loadDetails fetches ETH and token balances for an address.
func loadDetails(client *rpc.Client, addr common.Address, watch []rpc.WatchedToken) tea.Cmd {
	return func() tea.Msg {
		return detailsLoadedMsg{d: rpc.LoadWalletDetails(client, addr, watch)}
	}
}
