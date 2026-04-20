package main

import (
	"fmt"
	"math/big"
	"strings"

	"charm-wallet-tui/config"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleTerraClaimPopupMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.activeDialog = dialogNone
			m.terraNullMsgInput.Blur()
			m.terraNullMsgError = ""
			return m, nil
		case "tab", "down":
			if m.terraNullFormFocused == 0 {
				m.terraNullFormFocused = 1
				m.terraNullMsgInput.Blur()
			} else {
				m.terraNullFormFocused = 0
				return m, m.terraNullMsgInput.Focus()
			}
			return m, nil
		case "shift+tab", "up":
			if m.terraNullFormFocused == 1 {
				m.terraNullFormFocused = 0
				return m, m.terraNullMsgInput.Focus()
			} else {
				m.terraNullFormFocused = 1
				m.terraNullMsgInput.Blur()
			}
			return m, nil
		case "enter":
			if m.terraNullFormFocused == 0 {
				// Move to button
				m.terraNullFormFocused = 1
				m.terraNullMsgInput.Blur()
			} else {
				// Submit
				msgVal := strings.TrimSpace(m.terraNullMsgInput.Value())
				if msgVal == "" {
					m.terraNullMsgError = "must not be blank"
					m.terraNullFormFocused = 0
					return m, m.terraNullMsgInput.Focus()
				}
				m.terraNullMsgError = ""
				m.activeDialog = dialogNone
				m.terraNullMsgInput.Blur()
				m.addLog("info", fmt.Sprintf("Terra Nullius: packaging claim → \"%s\"", msgVal))
				m.activeDialog = dialogTxResult
				m.txResultPackaging = true
				m.txResultHex = ""
				m.txResultError = ""
				m.txResultFormat = "EIP-4527"
				return m, packageTerraClaimTx(m.activeAddress, msgVal, m.rpcURL)
			}
			return m, nil
		}
		// Pass all other keys to the textinput when input is focused
		if m.terraNullFormFocused == 0 {
			var cmd tea.Cmd
			m.terraNullMsgInput, cmd = m.terraNullMsgInput.Update(keyMsg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) handleTerraKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogScanTx {
		return m.handleScanTxKey(msg)
	}

	if m.activeDialog == dialogTxResult {
		var vpCmd tea.Cmd
		m.txQRViewport, vpCmd = m.txQRViewport.Update(msg)
		switch msg.String() {
		case "ctrl+c":
			if m.txResultHex != "" {
				m.addLog("info", "Copied EIP-4527 transaction to clipboard")
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

	switch msg.String() {
	case "esc":
		return m, m.navigateTo(config.PageDappBrowser)

	case "up", "k":
		if m.terraNullFocusedField > 1 {
			m.terraNullFocusedField--
		}
		return m, nil

	case "down", "j":
		if m.terraNullFocusedField < 2 {
			m.terraNullFocusedField++
		}
		return m, nil

	case "tab":
		if m.terraNullFocusedField == 1 {
			m.terraNullFocusedField = 2
		} else {
			m.terraNullFocusedField = 1
		}
		return m, nil

	case "shift+tab":
		if m.terraNullFocusedField == 2 {
			m.terraNullFocusedField = 1
		} else {
			m.terraNullFocusedField = 2
		}
		return m, nil

	case "enter":
		if m.terraNullFocusedField == 1 {
			// Execute claims(index) query
			if m.terraNullClaimInput == "" {
				m.terraNullClaimResultErr = "enter a claim index"
				return m, nil
			}
			idx := new(big.Int)
			_, ok := idx.SetString(m.terraNullClaimInput, 10)
			if !ok || idx.Sign() < 0 {
				m.terraNullClaimResultErr = "invalid index"
				return m, nil
			}
			m.terraNullClaimQuerying = true
			m.terraNullClaimResult = nil
			m.terraNullClaimResultErr = ""
			m.terraNullLastQueriedIdx = m.terraNullClaimInput
			m.addLog("info", fmt.Sprintf("Terra Nullius: querying claim #%s", m.terraNullClaimInput))
			m.terraNullClaimInput = new(big.Int).Add(idx, big.NewInt(1)).String()
			return m, fetchTerraClaim(m.ethClient, idx)
		} else if m.terraNullFocusedField == 2 {
			// Open claim popup
			m.activeDialog = dialogTerraClaim
			m.terraNullFormFocused = 0
			m.terraNullMsgInput.SetValue("")
			m.terraNullMsgError = ""
			return m, m.terraNullMsgInput.Focus()
		}
		return m, nil

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if m.terraNullFocusedField == 1 {
			m.terraNullClaimInput += msg.String()
			m.terraNullClaimResult = nil
			m.terraNullClaimResultErr = ""
		}
		return m, nil

	case "backspace":
		if m.terraNullFocusedField == 1 && len(m.terraNullClaimInput) > 0 {
			m.terraNullClaimInput = m.terraNullClaimInput[:len(m.terraNullClaimInput)-1]
			m.terraNullClaimResult = nil
			m.terraNullClaimResultErr = ""
		}
		return m, nil
	}
	return m, nil
}

