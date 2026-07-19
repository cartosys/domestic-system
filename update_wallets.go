package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) createSendForm() {
	tempSendToAddr = ""
	tempSendAmount = ""
	m.sendFormError = ""
	m.sendFormButtonFocused = false

	addrField := huh.NewInput().
		Title("Send To").
		Description("Enter a valid Ethereum address (Ctrl+v to paste)").
		Value(&tempSendToAddr).
		Placeholder("0x...").
		Validate(func(s string) error {
			if !helpers.IsValidEthAddress(s) {
				return fmt.Errorf("invalid ethereum address")
			}
			return nil
		})

	amountField := huh.NewInput().
		Title("Amount (ETH)").
		Description(fmt.Sprintf("Available: %s ETH", helpers.FormatETH(m.details.EthWei))).
		Value(&tempSendAmount).
		Placeholder("0.0").
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("amount is required")
			}
			// Parse amount as big.Float
			amount := new(big.Float)
			_, ok := amount.SetString(s)
			if !ok {
				return fmt.Errorf("invalid amount")
			}
			// Check if amount is <= balance
			balanceFloat := new(big.Float).SetInt(m.details.EthWei)
			balanceETH := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))
			if amount.Cmp(balanceETH) > 0 {
				return fmt.Errorf("amount exceeds balance")
			}
			if amount.Cmp(big.NewFloat(0)) <= 0 {
				return fmt.Errorf("amount must be greater than 0")
			}
			return nil
		})

	m.sendFormFields = []huh.Field{addrField, amountField}
	m.sendForm = huh.NewForm(
		huh.NewGroup(addrField, amountField),
	).WithWidth(SendFormPopupInnerWidth).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.sendForm.Init()
}

func (m *model) handleSendFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Belt-and-suspenders: drop mouse events that slipped past the Update() guard
	// (raw SGR sequences can arrive as tea.KeyMsg on some terminals).
	if _, ok := msg.(tea.MouseMsg); ok {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Intercept ESC key to cancel form
		if keyMsg.String() == "esc" {
			m.activeDialog = dialogNone
			m.sendForm = nil
			m.sendFormButtonFocused = false
			return m, cmdEnableMouseAllMotion()
		}

		// Read the clipboard ourselves, synchronously, and feed it in as
		// typed runes rather than letting huh's async Paste cmd run: in a
		// multi-field group, huh.Group.Update (v0.7.0) calls the focused
		// field's Update twice for any non-KeyMsg message — once via its
		// broadcast-to-all-fields branch, once via its focused-field branch
		// — which double-inserts the pasted text.
		if keyMsg.String() == "ctrl+v" && !m.sendFormButtonFocused {
			text, err := clipboard.ReadAll()
			switch {
			case err != nil:
				m.sendFormError = "Clipboard unavailable: " + err.Error()
				m.sendFormErrTime = time.Now()
				return m, nil
			case text == "":
				m.sendFormError = "Clipboard is empty"
				m.sendFormErrTime = time.Now()
				return m, nil
			default:
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
			}
		}

		lastField := m.sendFormFields[len(m.sendFormFields)-1]

		if m.sendFormButtonFocused {
			// Focus is on the Submit button, not a text field — handle the
			// button's own keys here rather than forwarding to huh.
			switch keyMsg.String() {
			case "enter", " ":
				return m.trySubmitSendForm()
			case "tab":
				m.sendFormButtonFocused = false
				return m, focusHuhField(m.sendForm, m.sendFormFields, 0)
			case "shift+tab":
				m.sendFormButtonFocused = false
				return m, focusHuhField(m.sendForm, m.sendFormFields, len(m.sendFormFields)-1)
			}
			return m, nil
		}

		// Tab on the last field would otherwise complete the form directly
		// (huh treats Tab/Enter on the last field of the only group as
		// submit); intercept it here so Tab stops on the Submit button first.
		if keyMsg.String() == "tab" && m.sendForm.GetFocusedField() == lastField {
			m.sendFormButtonFocused = true
			return m, lastField.Blur()
		}
	}

	form, cmd := m.sendForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.sendForm = f

		// Check if form is completed
		if m.sendForm.State == huh.StateCompleted {
			// Package the transaction
			m.logInfo(fmt.Sprintf("Packaging transaction: %s ETH to %s", tempSendAmount, helpers.ShortenAddr(tempSendToAddr)))
			m.sendForm = nil
			m.activeDialog = dialogTxResult
			m.txResultPackaging = true
			m.txResultHex = ""
			m.txResultError = ""
			m.txResultFormat = "EIP-4527"
			return m, tea.Batch(packageTransaction(m.activeAddress, tempSendToAddr, tempSendAmount, m.rpcURL), cmdEnableMouseAllMotion())
		}

		// Check if form was aborted (ESC pressed)
		if m.sendForm.State == huh.StateAborted {
			m.activeDialog = dialogNone
			m.sendForm = nil
			return m, cmdEnableMouseAllMotion()
		}
	}
	return m, cmd
}

// trySubmitSendForm validates the send form's current field values and, if
// valid, packages the transaction the same way completing the huh form does.
// Used by the mouse-clickable Submit button, since clicks never reach huh's
// own per-field validation (which only runs on a real Enter keypress).
func (m *model) trySubmitSendForm() (tea.Model, tea.Cmd) {
	addr := strings.TrimSpace(tempSendToAddr)
	if !helpers.IsValidEthAddress(addr) {
		m.sendFormError = "Invalid Ethereum Address"
		m.sendFormErrTime = time.Now()
		return m, nil
	}

	amtStr := strings.TrimSpace(tempSendAmount)
	if amtStr == "" {
		m.sendFormError = "Amount is required"
		m.sendFormErrTime = time.Now()
		return m, nil
	}
	amount := new(big.Float)
	if _, ok := amount.SetString(amtStr); !ok {
		m.sendFormError = "Invalid amount"
		m.sendFormErrTime = time.Now()
		return m, nil
	}
	balanceFloat := new(big.Float).SetInt(m.details.EthWei)
	balanceETH := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))
	if amount.Cmp(balanceETH) > 0 {
		m.sendFormError = "Amount exceeds balance"
		m.sendFormErrTime = time.Now()
		return m, nil
	}
	if amount.Cmp(big.NewFloat(0)) <= 0 {
		m.sendFormError = "Amount must be greater than 0"
		m.sendFormErrTime = time.Now()
		return m, nil
	}

	m.logInfo(fmt.Sprintf("Packaging transaction: %s ETH to %s", tempSendAmount, helpers.ShortenAddr(addr)))
	m.sendFormError = ""
	m.sendForm = nil
	m.activeDialog = dialogTxResult
	m.txResultPackaging = true
	m.txResultHex = ""
	m.txResultError = ""
	m.txResultFormat = "EIP-4527"
	return m, tea.Batch(packageTransaction(m.activeAddress, addr, tempSendAmount, m.rpcURL), cmdEnableMouseAllMotion())
}

// confirmDeleteWalletYes deletes the wallet pending confirmation. Shared by
// the keyboard Enter-on-Yes path and the dialog's mouse-clickable Yes button.
func (m *model) confirmDeleteWalletYes() (tea.Model, tea.Cmd) {
	idx := m.deleteDialogIdx
	deletedAddr := m.deleteDialogAddr
	m.accounts = append(m.accounts[:idx], m.accounts[idx+1:]...)
	if m.selectedWallet >= len(m.accounts) && m.selectedWallet > 0 {
		m.selectedWallet--
	}
	if len(m.accounts) > 0 {
		m.highlightedAddress = m.accounts[m.selectedWallet].Address
		m.activeAddress = ""
		for _, w := range m.accounts {
			if w.Active {
				m.activeAddress = w.Address
				break
			}
		}
	} else {
		m.highlightedAddress = ""
		m.activeAddress = ""
	}
	config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
	m.logWarn(fmt.Sprintf("Deleted wallet `%s`", helpers.ShortenAddr(deletedAddr)))
	m.activeDialog = dialogNone
	return m, m.loadSelectedWalletDetails()
}

// confirmDeleteWalletNo cancels the pending wallet deletion.
func (m *model) confirmDeleteWalletNo() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	return m, nil
}

// focusWalletFormField moves focus to the given field (0=address, 1=nickname)
// in the Add/Edit Wallet dialog. Used by the mouse click-to-focus regions —
// kept separate from the Tab-key handler, which also conditionally triggers
// an ENS lookup as part of advancing focus; clicking a field directly should
// just move focus, not replicate that side effect.
func (m *model) focusWalletFormField(idx int) {
	if idx == m.focusedInput {
		return
	}
	if idx == 0 {
		m.nicknameInput.Blur()
		m.input.Focus()
		m.focusedInput = 0
	} else {
		m.input.Blur()
		m.nicknameInput.Focus()
		m.focusedInput = 1
	}
}

func (m *model) handleWalletsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle delete confirmation dialog
	if m.activeDialog == dialogDeleteWallet {
		switch msg.String() {
		case "left", "right", "tab":
			// Toggle between Yes and No buttons
			m.deleteDialogYesSelected = !m.deleteDialogYesSelected
			return m, nil
		case "enter":
			// Execute based on selected button
			if m.deleteDialogYesSelected {
				return m.confirmDeleteWalletYes()
			}
			return m.confirmDeleteWalletNo()
		case "esc":
			// Cancel deletion
			m.activeDialog = dialogNone
			return m, nil
		}
		return m, nil
	}

	// Handle edit wallet dialog
	if m.activeDialog == dialogEditWallet {
		switch msg.String() {
		case "esc", "escape":
			m.activeDialog = dialogNone
			m.input.SetValue("")
			m.nicknameInput.SetValue("")
			m.input.Blur()
			m.nicknameInput.Blur()
			m.focusedInput = 0
			m.addError = ""
			m.ensLookupActive = false
			m.ensLookupAddr = ""
			return m, cmdEnableMouseAllMotion()

		case "ctrl+v":
			text, err := clipboard.ReadAll()
			if err == nil && text != "" {
				if m.focusedInput == 0 {
					m.input.SetValue(text)
				} else {
					m.nicknameInput.SetValue(text)
				}
			}
			return m, nil

		case "shift+tab", "tab", "ctrl+i", "down":
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())
				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
						m.ensLookupActive = true
						m.ensLookupAddr = newAddr
						m.focusedInput = 1
						m.input.Blur()
						m.nicknameInput.Focus()
						return m, lookupENS(m.ethClient, newAddr)
					}
				}
				m.focusedInput = 1
				m.input.Blur()
				m.nicknameInput.Focus()
			} else {
				m.focusedInput = 0
				m.nicknameInput.Blur()
				m.input.Focus()
			}
			return m, nil

		case "enter":
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())
				if strings.HasSuffix(strings.ToLower(val), ".eth") {
					if m.ethClient != nil {
						m.ensLookupActive = true
						m.ensLookupAddr = val
						return m, resolveENS(m.ethClient, val)
					}
					return m, nil
				}
				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					m.focusedInput = 1
					m.input.Blur()
					m.nicknameInput.Focus()
					if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
						m.ensLookupActive = true
						m.ensLookupAddr = newAddr
						return m, lookupENS(m.ethClient, newAddr)
					}
					return m, nil
				}
				return m, nil
			}
			// Nickname field — submit
			val := strings.TrimSpace(m.input.Value())
			if !helpers.IsValidEthAddress(val) {
				m.addError = "Invalid Ethereum Address"
				m.addErrTime = time.Now()
				m.focusedInput = 0
				m.nicknameInput.Blur()
				m.input.Focus()
				return m, nil
			}
			newAddr := common.HexToAddress(val).Hex()
			for i, w := range m.accounts {
				if i != m.editingIdx && strings.EqualFold(w.Address, newAddr) {
					m.addError = "Duplicate address - wallet already exists"
					m.addErrTime = time.Now()
					m.focusedInput = 0
					m.nicknameInput.Blur()
					m.input.Focus()
					return m, nil
				}
			}
			nickname := strings.TrimSpace(m.nicknameInput.Value())
			m.accounts[m.editingIdx].Address = newAddr
			m.accounts[m.editingIdx].Name = nickname
			if m.accounts[m.editingIdx].Active {
				m.activeAddress = newAddr
				m.highlightedAddress = newAddr
			}
			config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
			m.logSuccess(fmt.Sprintf("Updated wallet `%s`", helpers.ShortenAddr(newAddr)))
			m.activeDialog = dialogNone
			m.input.SetValue("")
			m.nicknameInput.SetValue("")
			m.input.Blur()
			m.nicknameInput.Blur()
			m.focusedInput = 0
			m.addError = ""
			m.ensLookupActive = false
			m.ensLookupAddr = ""
			return m, tea.Batch(m.loadSelectedWalletDetails(), cmdEnableMouseAllMotion())
		}

		var cmd tea.Cmd
		if m.focusedInput == 0 {
			m.input, cmd = m.input.Update(msg)
		} else {
			m.nicknameInput, cmd = m.nicknameInput.Update(msg)
		}
		return m, cmd
	}

	// Handle send button focus toggle with Tab

	switch msg.String() {
	case "tab":
		// Don't allow tab navigation when send form or add wallet form is active
		if m.activeDialog == dialogSendTx || m.activeDialog == dialogAddWallet {

		} else {
			// Only allow focusing send button if ETH balance > 0
			if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
				m.sendButtonFocused = !m.sendButtonFocused
			}
			return m, nil
		}

	case "enter":
		// Show send form when send button is focused
		if m.sendButtonFocused && m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
			m.createSendForm()
			m.activeDialog = dialogSendTx
			m.sendButtonFocused = false
			return m, cmdEnableMouseCellMotion()
		}
	}
	// Handle add wallet dialog
	if m.activeDialog == dialogAddWallet {
		switch msg.String() {
		case "esc", "escape":
			m.activeDialog = dialogNone
			m.input.SetValue("")
			m.nicknameInput.SetValue("")
			m.input.Blur()
			m.nicknameInput.Blur()
			m.focusedInput = 0
			m.addError = ""
			m.ensLookupActive = false
			m.ensLookupAddr = ""
			return m, cmdEnableMouseAllMotion()

		case "ctrl+v":
			text, err := clipboard.ReadAll()
			if err == nil && text != "" {
				if m.focusedInput == 0 {
					m.input.SetValue(text)
				} else {
					m.nicknameInput.SetValue(text)
				}
			}
			return m, nil

		case "shift+tab", "tab", "ctrl+i", "down":
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())
				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
						m.ensLookupActive = true
						m.ensLookupAddr = newAddr
						m.focusedInput = 1
						m.input.Blur()
						m.nicknameInput.Focus()
						return m, lookupENS(m.ethClient, newAddr)
					}
				}
				m.focusedInput = 1
				m.input.Blur()
				m.nicknameInput.Focus()
			} else {
				m.focusedInput = 0
				m.nicknameInput.Blur()
				m.input.Focus()
			}
			return m, nil

		case "enter":
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())
				if strings.HasSuffix(strings.ToLower(val), ".eth") {
					if m.ethClient != nil {
						m.ensLookupActive = true
						m.ensLookupAddr = val
						return m, resolveENS(m.ethClient, val)
					}
					return m, nil
				}
				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					m.focusedInput = 1
					m.input.Blur()
					m.nicknameInput.Focus()
					if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
						m.ensLookupActive = true
						m.ensLookupAddr = newAddr
						return m, lookupENS(m.ethClient, newAddr)
					}
					return m, nil
				}
				return m, nil
			}
			// Nickname field — submit
			val := strings.TrimSpace(m.input.Value())
			if !helpers.IsValidEthAddress(val) {
				m.addError = "Invalid Ethereum Address"
				m.addErrTime = time.Now()
				m.input.SetValue("")
				m.nicknameInput.SetValue("")
				m.focusedInput = 0
				m.input.Focus()
				return m, nil
			}
			newAddr := common.HexToAddress(val).Hex()
			for _, w := range m.accounts {
				if strings.EqualFold(w.Address, newAddr) {
					m.addError = "Duplicate address - wallet already exists"
					m.addErrTime = time.Now()
					m.input.SetValue("")
					m.nicknameInput.SetValue("")
					m.focusedInput = 0
					m.input.Focus()
					return m, nil
				}
			}
			nickname := strings.TrimSpace(m.nicknameInput.Value())
			newWallet := config.WalletEntry{Address: newAddr, Name: nickname, Active: false}
			m.accounts = append(m.accounts, newWallet)
			m.selectedWallet = len(m.accounts) - 1
			m.highlightedAddress = newAddr
			m.activeDialog = dialogNone
			m.input.SetValue("")
			m.nicknameInput.SetValue("")
			m.input.Blur()
			m.nicknameInput.Blur()
			m.focusedInput = 0
			m.addError = ""
			config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
			if nickname != "" {
				m.logSuccess(fmt.Sprintf("Added wallet `%s` with nickname `%s`", helpers.ShortenAddr(newAddr), nickname))
			} else {
				m.logSuccess(fmt.Sprintf("Added wallet `%s`", helpers.ShortenAddr(newAddr)))
			}
			return m, tea.Batch(m.loadSelectedWalletDetails(), cmdEnableMouseAllMotion())
		}

		var cmd tea.Cmd
		if m.focusedInput == 0 {
			m.input, cmd = m.input.Update(msg)
		} else {
			m.nicknameInput, cmd = m.nicknameInput.Update(msg)
		}
		return m, cmd
	}

	// normal list controls
	switch msg.String() {
	case "up", "k":
		if m.selectedWallet > 0 {
			m.selectedWallet--
			if len(m.accounts) > 0 {
				m.highlightedAddress = m.accounts[m.selectedWallet].Address
			}
		}
		return m, nil

	case "down", "j":
		if m.selectedWallet < len(m.accounts)-1 {
			m.selectedWallet++
			if len(m.accounts) > 0 {
				m.highlightedAddress = m.accounts[m.selectedWallet].Address
			}
		}
		return m, nil

	case "a", "A":
		m.focusedInput = 0
		m.input.SetValue("")
		m.nicknameInput.SetValue("")
		m.input.Focus()
		m.nicknameInput.Blur()
		m.addError = ""
		m.ensLookupActive = false
		m.ensLookupAddr = ""
		m.activeDialog = dialogAddWallet
		return m, cmdEnableMouseCellMotion()

	case "enter":
		// Set selected wallet as active
		if len(m.accounts) > 0 {
			for i := range m.accounts {
				m.accounts[i].Active = (i == m.selectedWallet)
			}
			// Update active address to the newly activated wallet
			m.activeAddress = m.accounts[m.selectedWallet].Address
			config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
			m.logInfo(fmt.Sprintf("Activated wallet `%s`", helpers.ShortenAddr(m.activeAddress)))

			// If split view is enabled, refresh details for the newly activated wallet
			if m.detailsInWallets {
				addr := m.accounts[m.selectedWallet].Address
				m.loading = true
				m.details = rpc.WalletDetails{Address: addr}
				ethAddr := common.HexToAddress(addr)
				return m, loadDetails(m.ethClient, ethAddr, m.tokenWatchForActiveChain())
			}
		}
		return m, nil

	case "e", "E":
		if len(m.accounts) > 0 {
			w := m.accounts[m.selectedWallet]
			m.editingIdx = m.selectedWallet
			m.input.SetValue(w.Address)
			m.nicknameInput.SetValue(w.Name)
			m.focusedInput = 0
			m.input.Focus()
			m.nicknameInput.Blur()
			m.addError = ""
			m.ensLookupActive = false
			m.ensLookupAddr = ""
			m.activeDialog = dialogEditWallet
			return m, cmdEnableMouseCellMotion()
		}
		return m, nil

	case "s", "S":
		return m, m.navigateTo(config.PageSettings)

	case "b", "B":
		return m, m.navigateTo(config.PageDappBrowser)

	case "w", "W":
		return m, m.navigateTo(config.PageWatchedTokens)

	case "h", "H":

	case "esc":
		return m, tea.Quit

	case "delete", "backspace":
		// Show delete confirmation dialog
		if len(m.accounts) == 0 {
			return m, nil
		}
		m.activeDialog = dialogDeleteWallet
		m.deleteDialogYesSelected = true // Default to Yes button
		m.deleteDialogIdx = m.selectedWallet
		m.deleteDialogAddr = m.accounts[m.selectedWallet].Address
		return m, nil
	}
	return m, nil
}
