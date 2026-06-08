package main

import (
	"fmt"
	"math/big"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) handleUniswapKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle transaction result panel first
	if m.activeDialog == dialogScanTx {
		return m.handleScanTxKey(msg)
	}

	if m.activeDialog == dialogTxResult {
		var vpCmd tea.Cmd
		m.txQRViewport, vpCmd = m.txQRViewport.Update(msg)
		switch msg.String() {
		case "ctrl+c":
			if m.txResultHex != "" {
				m.logInfo("Copied EIP-4527 transaction to clipboard")
				return m, tea.Batch(vpCmd, copyTxJsonToClipboard(m.txResultHex))
			}
			return m, vpCmd
		case "enter":
			return m.openScanTxDialog()
		case "esc":
			m.activeDialog = dialogNone
			m.txResultHex = ""
			m.txResultEIP681 = ""
			m.txResultError = ""
			m.txResultPackaging = false
			return m, nil
		}
		return m, vpCmd
	}

	// Handle liquidity positions view
	if m.uniswapShowingLiquidity {
		switch msg.String() {
		case "esc", "q", "Q":
			m.uniswapShowingLiquidity = false
			return m, nil
		case "up", "k":
			if m.liquidityFocusedIdx > 0 {
				m.liquidityFocusedIdx--
			}
			return m, nil
		case "down", "j":
			if m.liquidityFocusedIdx < len(m.liquidityPositions)-1 {
				m.liquidityFocusedIdx++
			}
			return m, nil
		case "l":
			m.logEnabled = !m.logEnabled
			return m, nil
		}
		return m, nil
	}

	// Handle token selector popup
	if m.uniswapShowingSelector {
		switch msg.String() {
		case "esc":
			m.uniswapShowingSelector = false
			return m, nil
		case "up", "k":
			if m.uniswapSelectorIdx > 0 {
				m.uniswapSelectorIdx--
			}
			return m, nil
		case "down", "j":
			// Build token list from wallet details
			tokens := m.buildTokenList()
			if m.uniswapSelectorIdx < len(tokens)-1 {
				m.uniswapSelectorIdx++
			}
			return m, nil
		case "enter":
			// Select token and close selector
			if m.uniswapSelectorFor == 0 {
				m.uniswapFromTokenIdx = m.uniswapSelectorIdx
			} else {
				m.uniswapToTokenIdx = m.uniswapSelectorIdx
			}
			m.uniswapShowingSelector = false
			// Trigger quote fetch since token selection changed
			return m, m.maybeRequestUniswapQuote()
		}
		return m, nil
	}

	// Main swap interface controls
	switch msg.String() {
	case "esc":
		if m.poolEventMonitor != nil {
			m.poolEventMonitor.Stop()
			m.poolEventMonitor = nil
			m.poolEventMonitorActive = false
		}
		return m, m.navigateTo(config.PageDappBrowser)

	case "up", "k":
		// Navigate up through fields
		if m.uniswapFocusedField > 0 {
			// If leaving To field with value, trigger reverse quote
			if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
				m.uniswapFocusedField--
				m.uniswapEditingFrom = false
				m.uniswapEditingTo = false
				return m, m.maybeRequestReverseUniswapQuote()
			}
			m.uniswapFocusedField--
			// Reset editing flags when navigating to a field
			if m.uniswapFocusedField == 0 {
				m.uniswapEditingFrom = false
			} else if m.uniswapFocusedField == 1 {
				m.uniswapEditingTo = false
			}
		}
		return m, nil

	case "down", "j":
		// Navigate down through fields
		if m.uniswapFocusedField < 2 {
			// If leaving From field, trigger forward quote
			if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
				m.uniswapFocusedField++
				if m.uniswapFocusedField == 1 {
					m.uniswapEditingTo = false
				}
				return m, m.maybeRequestUniswapQuote()
			}
			// If leaving To field, trigger reverse quote
			if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
				m.uniswapFocusedField++
				m.uniswapEditingTo = false
				return m, m.maybeRequestReverseUniswapQuote()
			}
			m.uniswapFocusedField++
			if m.uniswapFocusedField == 1 {
				m.uniswapEditingTo = false
			}
		}
		return m, nil

	case "tab":
		// Cycle through fields
		// If leaving From field, trigger forward quote
		if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
			m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
			// Reset editing flags when entering a field
			if m.uniswapFocusedField == 1 {
				m.uniswapEditingTo = false
			}
			return m, m.maybeRequestUniswapQuote()
		}
		// If leaving To field, trigger reverse quote
		if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
			m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
			m.uniswapEditingTo = false
			return m, m.maybeRequestReverseUniswapQuote()
		}
		m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
		// Reset editing flags when entering a field
		if m.uniswapFocusedField == 0 {
			m.uniswapEditingFrom = false
		} else if m.uniswapFocusedField == 1 {
			m.uniswapEditingTo = false
		}
		return m, nil

	case "shift+tab":
		// Cycle through fields in reverse
		// If leaving From field, trigger forward quote
		if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
			m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
			// Reset editing flags when entering a field
			if m.uniswapFocusedField == 1 {
				m.uniswapEditingTo = false
			}
			return m, m.maybeRequestUniswapQuote()
		}
		// If leaving To field, trigger reverse quote
		if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
			m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
			m.uniswapEditingFrom = false
			m.uniswapEditingTo = false
			return m, m.maybeRequestReverseUniswapQuote()
		}
		m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
		// Reset editing flags when entering a field
		if m.uniswapFocusedField == 0 {
			m.uniswapEditingFrom = false
		} else if m.uniswapFocusedField == 1 {
			m.uniswapEditingTo = false
		}
		return m, nil

	case "enter":
		if m.uniswapFocusedField == 0 {
			// If user has been editing, move to next field instead of opening selector
			if m.uniswapEditingFrom {
				if m.uniswapFromAmount != "" {
					m.uniswapFocusedField++
					m.uniswapEditingTo = false
					return m, m.maybeRequestUniswapQuote()
				}
				m.uniswapFocusedField++
				m.uniswapEditingTo = false
				return m, nil
			}
			// Otherwise, open token selector for "from" field
			var cmd tea.Cmd
			if m.uniswapFromAmount != "" {
				cmd = m.maybeRequestUniswapQuote()
			}
			m.uniswapShowingSelector = true
			m.uniswapSelectorFor = 0
			m.uniswapSelectorIdx = m.uniswapFromTokenIdx
			return m, cmd
		} else if m.uniswapFocusedField == 1 {
			// If user has been editing To field, move to next field and trigger reverse quote
			if m.uniswapEditingTo {
				if m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
					m.uniswapFocusedField++
					m.uniswapEditingTo = false
					return m, m.maybeRequestReverseUniswapQuote()
				}
				m.uniswapFocusedField++
				m.uniswapEditingTo = false
				return m, nil
			}
			// Otherwise, open token selector for "to" field
			m.uniswapShowingSelector = true
			m.uniswapSelectorFor = 1
			m.uniswapSelectorIdx = m.uniswapToTokenIdx
			return m, nil
		} else if m.uniswapFocusedField == 2 {
			// Execute swap - package transaction and show QR code
			if m.uniswapFromAmount == "" || m.uniswapToAmount == "" {
				m.logError("Please enter an amount and get a quote first")
				return m, nil
			}
			if m.uniswapQuote == nil {
				m.logError("Please get a swap quote first")
				return m, nil
			}

			tokens := m.buildTokenList()
			if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
				return m, nil
			}
			if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
				return m, nil
			}

			fromToken := tokens[m.uniswapFromTokenIdx]
			toToken := tokens[m.uniswapToTokenIdx]

			m.logInfo(fmt.Sprintf("Packaging swap: %s %s → %s %s", m.uniswapFromAmount, fromToken.Symbol, m.uniswapToAmount, toToken.Symbol))
			m.activeDialog = dialogTxResult
			m.txResultPackaging = true
			m.txResultHex = ""
			m.txResultError = ""
			m.txResultFormat = "EIP-4527"
			return m, packageSwapTransaction(m.activeAddress, fromToken, toToken, m.uniswapFromAmount, m.uniswapQuote.AmountOut, m.rpcURL, m.chainID())
		}
		return m, nil

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ".":
		if m.uniswapFocusedField == 0 {
			v, e, ok := appendNumericInput(m.uniswapFromAmount, m.uniswapEditingFrom, msg.String())
			if !ok {
				return m, nil
			}
			m.uniswapFromAmount, m.uniswapEditingFrom = v, e
			return m, nil
		} else if m.uniswapFocusedField == 1 {
			v, e, ok := appendNumericInput(m.uniswapToAmount, m.uniswapEditingTo, msg.String())
			if !ok {
				return m, nil
			}
			m.uniswapToAmount, m.uniswapEditingTo = v, e
			return m, nil
		}
		return m, nil

	case "backspace":
		// Delete last character from amount
		if m.uniswapFocusedField == 0 && len(m.uniswapFromAmount) > 0 {
			m.uniswapFromAmount = m.uniswapFromAmount[:len(m.uniswapFromAmount)-1]
			m.uniswapEditingFrom = true // Mark that user is actively editing
			// Quote will be fetched when user leaves the field
			return m, nil
		} else if m.uniswapFocusedField == 1 && len(m.uniswapToAmount) > 0 {
			m.uniswapToAmount = m.uniswapToAmount[:len(m.uniswapToAmount)-1]
			m.uniswapEditingTo = true // Mark that user is actively editing
			return m, nil
		}
		return m, nil

	case "m", "M":
		// Max: populate From field with full balance
		if m.uniswapFocusedField == 0 {
			tokens := m.buildTokenList()
			if m.uniswapFromTokenIdx >= 0 && m.uniswapFromTokenIdx < len(tokens) {
				fromToken := tokens[m.uniswapFromTokenIdx]
				if fromToken.Balance != nil && fromToken.Balance.Sign() > 0 {
					// Convert balance to decimal string
					divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
					balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(fromToken.Balance), divisor)
					m.uniswapFromAmount = balanceFloat.Text('f', 6)
					m.uniswapEditingFrom = true // Mark that user is actively editing
					// Trigger quote fetch immediately for max
					m.logInfo(fmt.Sprintf("Max balance: %s %s", m.uniswapFromAmount, fromToken.Symbol))
					return m, m.maybeRequestUniswapQuote()
				}
			}
		}
		return m, nil

	case "q", "Q":
		m.uniswapShowingLiquidity = true
		m.liquidityFocusedIdx = 0
		if m.activeAddress != "" && m.rpcURL != "" {
			m.liquidityLoading = true
			m.liquidityPositions = nil
			m.liquidityErr = ""
			m.logInfo(fmt.Sprintf("Fetching V4 liquidity positions for %s…", helpers.ShortenAddr(m.activeAddress)))
			return m, fetchLiquidityPositions(m.rpcURL, common.HexToAddress(m.activeAddress))
		}
		return m, nil

	case "p", "P":
		if m.poolEventMonitorActive {
			// Stop the monitor
			if m.poolEventMonitor != nil {
				m.poolEventMonitor.Stop()
				m.poolEventMonitor = nil
			}
			m.poolEventMonitorActive = false
			m.logInfo("Pool Event Monitor stopped")
		} else {
			// Start the monitor
			monitor := helpers.NewPoolEventMonitor()
			monitor.Start(m.rpcURL)
			m.poolEventMonitor = monitor
			m.poolEventMonitorActive = true
			m.focusedPanel = focusedPanelV4Events
			m.logInfo("Pool Event Monitor starting… (requires wss:// RPC endpoint)")
			var startCmds []tea.Cmd
			startCmds = append(startCmds, waitForPoolEvent(monitor), waitForPoolEventData(monitor))
			if m.eventStore != nil {
				startCmds = append(startCmds, loadV4PoolTableCmd(m.eventStore))
			}
			return m, tea.Batch(startCmds...)
		}
		return m, nil

	case "b", "B":
		if m.v4BlockScanActive {
			if m.v4BlockScanner != nil {
				m.v4BlockScanner.Stop()
				m.v4BlockScanner = nil
			}
			m.v4BlockScanActive = false
			m.logInfo("V4 block scan cancelled")
		} else {
			scanner := helpers.NewV4BlockScanner()
			scanner.Start(m.rpcURL, 24686488, common.HexToAddress("0x5857bCe5490545a89598b9992DD0D409C4C20d86"))
			m.v4BlockScanner = scanner
			m.v4BlockScanActive = true
			m.logInfo("V4 block scan started (block 24686488)…")
			return m, waitForV4BlockScanLine(scanner)
		}
		return m, nil
	}
	return m, nil
}
