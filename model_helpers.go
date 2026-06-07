package main

import (
	"fmt"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/signer"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

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
	return m.adding ||
		(m.showSendForm && m.sendForm != nil) ||
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
	case config.PageSigner:
		m.signerDecoded = nil
		m.signerResult = nil
		m.signerSignErr = ""
		m.signerScanMode = false
		return loadSignerKeys()
	}
	return nil
}

// loadSignerKeys reads the private-keys file, bootstrapping defaults when absent.
func loadSignerKeys() tea.Cmd {
	return func() tea.Msg {
		keys, err := signer.LoadKeys()
		return signerKeysLoadedMsg{keys: keys, err: err}
	}
}

// signEIP4527 decodes a ur:eth-sign-request UR and signs it with the matching stored key.
func signEIP4527(urText string, keys []signer.KeyEntry) tea.Cmd {
	return func() tea.Msg {
		tx, err := signer.DecodeEIP4527UR(urText)
		if err != nil {
			return signerSignedMsg{err: fmt.Errorf("decode: %w", err)}
		}
		privKey := signer.FindKey(tx.From, keys)
		if privKey == "" {
			return signerSignedMsg{decoded: &tx, err: fmt.Errorf("no key stored for %s", tx.From)}
		}
		result, err := signer.SignTx(tx, privKey)
		if err != nil {
			return signerSignedMsg{decoded: &tx, err: err}
		}
		return signerSignedMsg{decoded: &tx, result: &result}
	}
}

// loadDetails fetches ETH and token balances for an address.
func loadDetails(client *rpc.Client, addr common.Address, watch []rpc.WatchedToken) tea.Cmd {
	return func() tea.Msg {
		return detailsLoadedMsg{d: rpc.LoadWalletDetails(client, addr, watch)}
	}
}
