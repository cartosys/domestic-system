package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) createSendForm() {
	tempSendToAddr = ""
	tempSendAmount = ""

	m.sendForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Send To").
				Description("Enter a valid Ethereum address (Ctrl+v to paste)").
				Value(&tempSendToAddr).
				Placeholder("0x...").
				Validate(func(s string) error {
					if !helpers.IsValidEthAddress(s) {
						return fmt.Errorf("invalid ethereum address")
					}
					return nil
				}),

			huh.NewInput().
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
				}),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.sendForm.Init()
}

func (m *model) handleSendFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept ESC key to cancel form
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.showSendForm = false
		m.sendForm = nil
		return m, cmdEnableMouseAllMotion()
	}

	form, cmd := m.sendForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.sendForm = f

		// Check if form is completed
		if m.sendForm.State == huh.StateCompleted {
			// Package the transaction
			m.addLog("info", fmt.Sprintf("Packaging transaction: %s ETH to %s", tempSendAmount, helpers.ShortenAddr(tempSendToAddr)))
			m.showSendForm = false
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
			m.showSendForm = false
			m.sendForm = nil
			return m, cmdEnableMouseAllMotion()
		}
	}
	return m, cmd
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
				// Confirm deletion (Yes button)
				idx := m.deleteDialogIdx
				deletedAddr := m.deleteDialogAddr
				m.accounts = append(m.accounts[:idx], m.accounts[idx+1:]...)
				// Update selected index
				if m.selectedWallet >= len(m.accounts) && m.selectedWallet > 0 {
					m.selectedWallet--
				}
				// Update highlighted address and check if active was deleted
				if len(m.accounts) > 0 {
					m.highlightedAddress = m.accounts[m.selectedWallet].Address
					// Update active address if needed
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
				// Save wallets to config
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
				m.addLog("warning", fmt.Sprintf("Deleted wallet `%s`", helpers.ShortenAddr(deletedAddr)))
				m.activeDialog = dialogNone
				// Load details for the newly selected wallet if split view is enabled
				return m, m.loadSelectedWalletDetails()
			}
			// Cancel deletion (No button)
			m.activeDialog = dialogNone
			return m, nil
		case "esc":
			// Cancel deletion
			m.activeDialog = dialogNone
			return m, nil
		}
		return m, nil
	}

	// Handle send button focus toggle with Tab

	switch msg.String() {
	case "tab":
		// Don't allow tab navigation when send form or add wallet form is active
		if m.showSendForm || m.adding {

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
			m.showSendForm = true
			m.sendButtonFocused = false
			return m, cmdEnableMouseCellMotion()
		}
	}
	// adding flow
	if m.adding {
		switch msg.String() {
		case "esc", "escape":
			// Cancel adding mode
			m.adding = false
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
			// Handle Ctrl+v paste explicitly to active input
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
			// Toggle between address and nickname fields
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())
				// Trigger ENS lookup if valid address
				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					// Trigger ENS lookup if connected and not already looking up this address
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
			// If on address field, check for .eth name or valid address
			if m.focusedInput == 0 {
				val := strings.TrimSpace(m.input.Value())

				// Check if it's a .eth ENS name
				if strings.HasSuffix(strings.ToLower(val), ".eth") {
					// Trigger forward ENS resolution
					if m.ethClient != nil {
						m.ensLookupActive = true
						m.ensLookupAddr = val // Store the ENS name being resolved
						return m, resolveENS(m.ethClient, val)
					}
					return m, nil
				}

				if helpers.IsValidEthAddress(val) {
					newAddr := common.HexToAddress(val).Hex()
					// Move to nickname field
					m.focusedInput = 1
					m.input.Blur()
					m.nicknameInput.Focus()
					// Trigger ENS reverse lookup if connected and not already looking up this address
					if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
						m.ensLookupActive = true
						m.ensLookupAddr = newAddr
						return m, lookupENS(m.ethClient, newAddr)
					}
					return m, nil
				}
				return m, nil
			}
			// If on nickname field, submit the form
			val := strings.TrimSpace(m.input.Value())
			if helpers.IsValidEthAddress(val) {
				newAddr := common.HexToAddress(val).Hex()

				// Check for duplicates
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

				// Create new wallet entry with nickname
				nickname := strings.TrimSpace(m.nicknameInput.Value())
				newWallet := config.WalletEntry{
					Address: newAddr,
					Name:    nickname,
					Active:  false,
				}
				m.accounts = append(m.accounts, newWallet)
				m.selectedWallet = len(m.accounts) - 1
				m.highlightedAddress = newAddr
				m.adding = false
				m.input.SetValue("")
				m.nicknameInput.SetValue("")
				m.input.Blur()
				m.nicknameInput.Blur()
				m.focusedInput = 0
				m.addError = ""
				// Save wallets to config
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
				if nickname != "" {
					m.addLog("success", fmt.Sprintf("Added wallet `%s` with nickname `%s`", helpers.ShortenAddr(newAddr), nickname))
				} else {
					m.addLog("success", fmt.Sprintf("Added wallet `%s`", helpers.ShortenAddr(newAddr)))
				}
				// Load details for the newly added wallet if split view is enabled
				return m, tea.Batch(m.loadSelectedWalletDetails(), cmdEnableMouseAllMotion())
			} else {
				m.addError = "Invalid Etherem Address"
				m.addErrTime = time.Now()
				m.input.SetValue("")
				m.nicknameInput.SetValue("")
				m.focusedInput = 0
				m.input.Focus()
				return m, nil
			}
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
		m.adding = true
		m.focusedInput = 0
		m.input.SetValue("")
		m.nicknameInput.SetValue("")
		m.input.Focus()
		m.nicknameInput.Blur()
		m.addError = ""
		m.ensLookupActive = false
		m.ensLookupAddr = ""
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
			m.addLog("info", fmt.Sprintf("Activated wallet `%s`", helpers.ShortenAddr(m.activeAddress)))

			// If split view is enabled, refresh details for the newly activated wallet
			if m.detailsInWallets {
				addr := m.accounts[m.selectedWallet].Address
				m.loading = true
				m.details = config.WalletDetails{Address: addr}
				ethAddr := common.HexToAddress(addr)
				return m, loadDetails(m.ethClient, ethAddr, m.tokenWatch)
			}
		}
		return m, nil

	case "s", "S":
		return m, m.navigateTo(config.PageSettings)

	case "b", "B":
		return m, m.navigateTo(config.PageDappBrowser)

	case "x", "X":
		return m, m.navigateTo(config.PageSigner)

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
