package main

import (
	"math/big"
	"sort"
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
		// helpers.OndoLiquidTokens (vendored by cmd/discoverondoliquidity) is
		// mainnet-only, matching SPCXon above — gate on the same "is this
		// mainnet" signal rather than adding a second network check.
		for _, t := range helpers.OndoLiquidTokens {
			if t.Address == addrs.SPCXon {
				continue // already added above
			}
			tokens = append(tokens, rpc.WatchedToken{Symbol: t.Symbol, Name: t.Name, Decimals: t.Decimals, Address: t.Address})
		}
	}
	return tokens
}

// tokenWatchToConfigList converts the in-memory watchlist to its persisted form.
func tokenWatchToConfigList(watch []rpc.WatchedToken) []config.WatchedToken {
	out := make([]config.WatchedToken, len(watch))
	for i, t := range watch {
		out[i] = config.WatchedToken{Symbol: t.Symbol, Name: t.Name, Decimals: t.Decimals, Address: t.Address.Hex(), TotalSupply: t.TotalSupply}
	}
	return out
}

// configListToTokenWatch converts the persisted watchlist to its in-memory form.
func configListToTokenWatch(entries []config.WatchedToken) []rpc.WatchedToken {
	out := make([]rpc.WatchedToken, len(entries))
	for i, e := range entries {
		out[i] = rpc.WatchedToken{Symbol: e.Symbol, Name: e.Name, Decimals: e.Decimals, Address: common.HexToAddress(e.Address), TotalSupply: e.TotalSupply}
	}
	return out
}

// sortedWatchedTokens returns watch ordered by the active wallet's raw on-chain
// balance for each token, highest first. Tokens with no loaded balance sort last;
// ties keep their original watchlist order.
func sortedWatchedTokens(watch []rpc.WatchedToken, details rpc.WalletDetails) []rpc.WatchedToken {
	balanceFor := func(addr common.Address) *big.Int {
		for _, tb := range details.Tokens {
			if tb.Address == addr {
				return tb.Balance
			}
		}
		return nil
	}

	sorted := make([]rpc.WatchedToken, len(watch))
	copy(sorted, watch)
	sort.SliceStable(sorted, func(i, j int) bool {
		bi := balanceFor(sorted[i].Address)
		bj := balanceFor(sorted[j].Address)
		if bi == nil {
			bi = big.NewInt(0)
		}
		if bj == nil {
			bj = big.NewInt(0)
		}
		return bi.Cmp(bj) > 0
	})
	return sorted
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
		((m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil) ||
		((m.tokenFormMode == "add" || m.tokenFormMode == "edit") && m.tokenForm != nil) ||
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

// loadSelectedWalletDetailsFresh drops the cached details for the selected
// wallet before reloading, so a just-added/edited watched token's balance is
// picked up immediately instead of being masked by a stale cache hit.
func (m *model) loadSelectedWalletDetailsFresh() tea.Cmd {
	if len(m.accounts) == 0 {
		return nil
	}
	addr := m.accounts[m.selectedWallet].Address
	delete(m.detailsCache, strings.ToLower(addr))
	return m.loadSelectedWalletDetails()
}

// navigateTo sets the active page and issues any initial Cmds required for that page.
func (m *model) navigateTo(page config.Page) tea.Cmd {
	m.activePage = page
	switch page {
	case config.PageWallets:
		return m.loadSelectedWalletDetails()
	case config.PageSettings:
		m.settingsMode = "list"
	case config.PageWatchedTokens:
		m.tokenFormMode = "list"
		m.selectedTokenIdx = 0
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
