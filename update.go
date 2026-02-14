package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/ethereum/go-ethereum/common"
)

// -------------------- TEMP FORM STORAGE --------------------
// Temporary form field storage (package-level to avoid pointer-to-copy issues)
var (
	tempRPCFormName   string
	tempRPCFormURL    string
	tempNicknameField string
	tempDappName      string
	tempDappAddress   string
	tempDappIcon      string
	tempDappNetwork   string
	tempSendToAddr    string
	tempSendAmount    string
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

func (m *model) createAddRPCForm() {
	tempRPCFormName = ""
	tempRPCFormURL = ""

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("RPC Name").
				Description("A friendly name for this RPC endpoint").
				Value(&tempRPCFormName).
				Placeholder("My Infura Node"),

			huh.NewInput().
				Title("RPC URL").
				Description("The complete RPC URL (https://...)").
				Value(&tempRPCFormURL).
				Placeholder("https://mainnet.infura.io/v3/..."),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createEditRPCForm(idx int) {
	if idx < 0 || idx >= len(m.rpcURLs) {
		return
	}

	rpc := m.rpcURLs[idx]
	tempRPCFormName = rpc.Name
	tempRPCFormURL = rpc.URL

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("RPC Name").
				Value(&tempRPCFormName).
				Placeholder("My Node"),

			huh.NewInput().
				Title("RPC URL").
				Value(&tempRPCFormURL).
				Placeholder("https://..."),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createNicknameForm() {
	// Find current wallet's nickname
	tempNicknameField = ""
	for _, w := range m.accounts {
		if strings.EqualFold(w.Address, m.details.Address) {
			tempNicknameField = w.Name
			break
		}
	}

	placeholderText := "Enter nickname"
	if tempNicknameField != "" {
		placeholderText = tempNicknameField
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Wallet Nickname").
				Description("Set a friendly name for this wallet").
				Value(&tempNicknameField).
				Placeholder(placeholderText),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createAddDappForm() {
	tempDappName = ""
	tempDappAddress = ""
	tempDappIcon = ""
	tempDappNetwork = ""

	// Build network options from RPC URLs
	networkOptions := []huh.Option[string]{}
	for _, rpcURL := range m.rpcURLs {
		networkOptions = append(networkOptions, huh.NewOption(rpcURL.Name, rpcURL.Name))
	}

	// Find the active RPC URL name as default
	defaultNetwork := ""
	for _, rpcURL := range m.rpcURLs {
		if rpcURL.Active {
			defaultNetwork = rpcURL.Name
			break
		}
	}
	tempDappNetwork = defaultNetwork

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("dApp Name").
				Description("A friendly name for this dApp").
				Value(&tempDappName).
				Placeholder("Uniswap"),

			huh.NewInput().
				Title("dApp Address").
				Description("The URL or address of the dApp").
				Value(&tempDappAddress).
				Placeholder("https://app.uniswap.org"),

			huh.NewInput().
				Title("Icon").
				Description("Icon or emoji for the dApp (optional)").
				Value(&tempDappIcon).
				Placeholder("🦄"),

			huh.NewSelect[string]().
				Options(networkOptions...).
				Title("Network").
				Description("Choose the network for this dApp").
				Value(&tempDappNetwork),
		),
	).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

func (m *model) createEditDappForm(idx int) {
	if idx < 0 || idx >= len(m.dapps) {
		return
	}

	dapp := m.dapps[idx]
	tempDappName = dapp.Name
	tempDappAddress = dapp.Address
	tempDappIcon = dapp.Icon
	tempDappNetwork = dapp.Network

	// Build network options from RPC URLs
	networkOptions := []huh.Option[string]{}
	for _, rpcURL := range m.rpcURLs {
		networkOptions = append(networkOptions, huh.NewOption(rpcURL.Name, rpcURL.Name))
	}

	// If current network is empty, use active RPC URL as default
	if tempDappNetwork == "" {
		for _, rpcURL := range m.rpcURLs {
			if rpcURL.Active {
				tempDappNetwork = rpcURL.Name
				break
			}
		}
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("dApp Name").
				Value(&tempDappName).
				Placeholder("Uniswap"),

			huh.NewInput().
				Title("dApp Address").
				Value(&tempDappAddress).
				Placeholder("https://app.uniswap.org"),

			huh.NewInput().
				Title("Icon").
				Value(&tempDappIcon).
				Placeholder("🦄"),

			huh.NewSelect[string]().
				Options(networkOptions...).
				Title("Network").
				Description("Choose the network for this dApp").
				Value(&tempDappNetwork),
		),
	).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

// -------------------- UPDATE --------------------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle send form updates first
	if m.activePage == config.PageWallets && m.showSendForm && m.sendForm != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.showSendForm = false
			m.sendForm = nil
			return m, nil
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
				m.showTxResultPanel = true
				m.txResultPackaging = true
				m.txResultHex = ""
				m.txResultError = ""
				m.txResultFormat = "EIP-681"
				return m, packageTransaction(m.activeAddress, tempSendToAddr, tempSendAmount, m.rpcURL)
			}

			// Check if form was aborted (ESC pressed)
			if m.sendForm.State == huh.StateAborted {
				m.showSendForm = false
				m.sendForm = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if m.activePage == config.PageHome {
		// TODO: home view not implemented yet
		// Temporarily disabled until home view is created
		m.activePage = config.PageWallets
		return m, m.loadSelectedWalletDetails()
	}

	// Handle form updates first (before message switching)
	if m.activePage == config.PageWallets && m.showSendForm && m.sendForm != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.showSendForm = false
			m.sendForm = nil
			return m, nil
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
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.sendForm.State == huh.StateAborted {
				m.showSendForm = false
				m.sendForm = nil
				return m, nil
			}
		}
		return m, cmd
	}

	// Handle form updates first (before message switching)
	if m.activePage == config.PageDetails && m.nicknaming && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.nicknaming = false
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				// Save nickname to wallet entry
				for i := range m.accounts {
					if strings.EqualFold(m.accounts[i].Address, m.details.Address) {
						oldName := m.accounts[i].Name
						m.accounts[i].Name = strings.TrimSpace(tempNicknameField)
						if oldName == "" && m.accounts[i].Name != "" {
							m.addLog("success", fmt.Sprintf("Set nickname `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
						} else if m.accounts[i].Name == "" {
							m.addLog("info", fmt.Sprintf("Cleared nickname for wallet `%s`", helpers.ShortenAddr(m.details.Address)))
						} else {
							m.addLog("success", fmt.Sprintf("Updated nickname to `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
						}
						break
					}
				}
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
				m.nicknaming = false
				m.form = nil
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.nicknaming = false
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if (m.activePage == config.PageDetails || m.activePage == config.PageDappBrowser) && (m.dappMode == "add" || m.dappMode == "edit") && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.dappMode = "list"
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				if m.dappMode == "add" {
					if tempDappName != "" && tempDappAddress != "" {
						newDapp := config.DApp{Name: tempDappName, Address: tempDappAddress, Icon: tempDappIcon, Network: tempDappNetwork}
						m.dapps = append(m.dapps, newDapp)
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Added dApp: `%s`", tempDappName))
					}
				} else if m.dappMode == "edit" {
					if m.selectedDappIdx >= 0 && m.selectedDappIdx < len(m.dapps) {
						m.dapps[m.selectedDappIdx].Name = tempDappName
						m.dapps[m.selectedDappIdx].Address = tempDappAddress
						m.dapps[m.selectedDappIdx].Icon = tempDappIcon
						m.dapps[m.selectedDappIdx].Network = tempDappNetwork
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Updated dApp: `%s`", tempDappName))
					}
				}
				m.dappMode = "list"
				m.form = nil
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.dappMode = "list"
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if m.activePage == config.PageSettings && (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.settingsMode = "list"
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				if m.settingsMode == "add" {
					if tempRPCFormName != "" && tempRPCFormURL != "" {
						newRPC := config.RPCUrl{Name: tempRPCFormName, URL: tempRPCFormURL, Active: false}
						m.rpcURLs = append(m.rpcURLs, newRPC)
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Added RPC endpoint: `%s` (%s)", tempRPCFormName, tempRPCFormURL))
					}
				} else if m.settingsMode == "edit" {
					if m.selectedRPCIdx >= 0 && m.selectedRPCIdx < len(m.rpcURLs) {
						m.rpcURLs[m.selectedRPCIdx].Name = tempRPCFormName
						m.rpcURLs[m.selectedRPCIdx].URL = tempRPCFormURL
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Updated RPC endpoint: `%s`", tempRPCFormName))
					}
				}
				m.settingsMode = "list"
				m.form = nil
				// Return without the form's cmd to ensure we're back in list mode
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.settingsMode = "list"
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	switch msg := msg.(type) {

	case logInitMsg:
		if !m.logEnabled {
			return m, nil
		}
		// Create logger that writes to our buffer
		m.logger = log.NewWithOptions(m.logBuffer, log.Options{
			ReportTimestamp: true,
			TimeFormat:      "15:04:05",
			Prefix:          "",
		})
		// Set log level and styling
		m.logger.SetLevel(log.DebugLevel)
		m.logger.SetStyles(&log.Styles{
			Timestamp: lipgloss.NewStyle().Foreground(cMuted),
			Caller:    lipgloss.NewStyle().Faint(true),
			Prefix:    lipgloss.NewStyle().Bold(true).Foreground(cAccent2),
			Message:   lipgloss.NewStyle().Foreground(cText),
			Key:       lipgloss.NewStyle().Foreground(cAccent),
			Value:     lipgloss.NewStyle().Foreground(cText),
			Separator: lipgloss.NewStyle().Faint(true),
			Levels: map[log.Level]lipgloss.Style{
				log.DebugLevel: lipgloss.NewStyle().Foreground(cMuted).SetString("DEBUG"),
				log.InfoLevel:  lipgloss.NewStyle().Foreground(cAccent2).SetString("INFO"),
				log.WarnLevel:  lipgloss.NewStyle().Foreground(cWarn).SetString("WARN"),
				log.ErrorLevel: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).SetString("ERROR"),
			},
		})
		m.logReady = true
		m.addLog("info", "Logger enabled")
		return m, nil

	case rpcConnectedMsg:
		m.rpcConnecting = false
		if msg.err != nil {
			// Connection failed
			m.ethClient = nil
			m.rpcConnected = false
			m.addLog("error", fmt.Sprintf("RPC connection failed: `%s`", msg.err.Error()))
		} else {
			// Connection successful
			m.ethClient = msg.client
			m.rpcConnected = true
			m.addLog("success", fmt.Sprintf("RPC connected to `%s`", msg.client.URL))
			// Load active account details automatically when on wallet page with split view
			if m.activePage == config.PageWallets && m.detailsInWallets && len(m.accounts) > 0 {
				return m, m.loadSelectedWalletDetails()
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

		// Only initialize viewport if log is enabled
		if m.logEnabled {
			// Update log viewport dimensions
			// Width accounts for border and padding
			m.logViewport.Width = max(0, msg.Width-6)
			// Height will be calculated dynamically in renderLogPanel
			if m.logReady {
				m.updateLogViewport()
			}
		}

		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		var cmds []tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
		// Update log spinner too if log is enabled but not ready
		if m.logEnabled && !m.logReady {
			m.logSpinner, cmd = m.logSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case detailsLoadedMsg:
		m.loading = false
		m.details = msg.d
		// Cache the loaded details
		if m.details.Address != "" {
			m.detailsCache[strings.ToLower(m.details.Address)] = m.details
		}
		if msg.err != nil && m.details.ErrMessage == "" {
			m.details.ErrMessage = "Failed to load wallet details."
			m.addLog("error", fmt.Sprintf("Failed to load details for `%s`", helpers.ShortenAddr(m.details.Address)))
		} else if m.details.ErrMessage != "" {
			m.addLog("error", fmt.Sprintf("Wallet `%s`: %s", helpers.ShortenAddr(m.details.Address), m.details.ErrMessage))
		} else {
			m.addLog("success", fmt.Sprintf("Loaded details for `%s` - ETH: %s", helpers.ShortenAddr(m.details.Address), helpers.FormatETH(m.details.EthWei)))
		}
		return m, nil

	case packageTransactionMsg:
		m.txResultPackaging = false
		if msg.err != nil {
			m.txResultError = msg.err.Error()
			m.addLog("error", "Transaction packaging failed: "+msg.err.Error())
		} else {
			m.txResultHex = msg.txDisplay
			m.txResultEIP681 = msg.qrData
			m.txResultFormat = msg.format
			m.addLog("success", "Transaction packaged successfully ("+msg.format+")")
		}
		return m, nil

	case tea.KeyMsg:
		// Handle transaction result panel FIRST (before any other keys)
		if m.showTxResultPanel {
			switch msg.String() {
			case "ctrl+c":
				// Copy transaction JSON to clipboard
				if m.txResultHex != "" {
					if m.txResultFormat == "EIP-681" {
						m.addLog("info", "Copied EIP-681 URL to clipboard")
					} else {
						m.addLog("info", "Copied transaction JSON to clipboard")
					}
					return m, copyTxJsonToClipboard(m.txResultHex)
				}
				return m, nil
			case "esc", "enter":
				m.showTxResultPanel = false
				m.txResultHex = ""
				m.txResultEIP681 = ""
				m.txResultError = ""
				m.txResultPackaging = false
				m.txResultFormat = ""
				m.txCopiedMsg = ""
				return m, nil
			}
			return m, nil
		}

		// Handle account list popup
		if m.showAccountListPopup {
			switch msg.String() {
			case "up", "k":
				if m.accountListSelectedIdx > 0 {
					m.accountListSelectedIdx--
				}
				return m, nil
			case "down", "j":
				if m.accountListSelectedIdx < len(m.accounts)-1 {
					m.accountListSelectedIdx++
				}
				return m, nil
			case "enter":
				// Activate the selected account
				if m.accountListSelectedIdx >= 0 && m.accountListSelectedIdx < len(m.accounts) {
					selectedAddr := m.accounts[m.accountListSelectedIdx].Address
					// Mark all as inactive
					for i := range m.accounts {
						m.accounts[i].Active = false
					}
					// Mark selected as active
					m.accounts[m.accountListSelectedIdx].Active = true
					m.activeAddress = selectedAddr
					m.highlightedAddress = selectedAddr
					m.selectedWallet = m.accountListSelectedIdx
					// Save config
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					m.addLog("success", fmt.Sprintf("Activated account: %s", helpers.ShortenAddr(selectedAddr)))
					// Close popup
					m.showAccountListPopup = false
					// Load details for newly activated wallet
					return m, m.loadSelectedWalletDetails()
				}
				return m, nil
			case "esc":
				m.showAccountListPopup = false
				return m, nil
			}
			return m, nil
		}

		allowMenuHotkeys := !m.textInputActive()
		// global keys
		if allowMenuHotkeys {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit

			case "l", "L":
				// Toggle logger
				m.logEnabled = !m.logEnabled
				if m.logEnabled {
					// Initialize viewport when enabling
					if m.w > 0 {
						m.logViewport.Width = m.w - 6
					}
					m.logReady = false
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					return m, tea.Batch(initLogViewport(), m.logSpinner.Tick)
				}
				// Clear logs and de-initialize when disabling
				if m.logBuffer != nil {
					m.logBuffer.Reset()
				}
				m.logger = nil
				m.logReady = false
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
				return m, nil

			case "pageup", "pagedown":
				// Allow scrolling in log viewport when enabled
				if m.logEnabled && m.logReady {
					var cmd tea.Cmd
					m.logViewport, cmd = m.logViewport.Update(msg)
					return m, cmd
				}
			}
		}

		// page-specific behavior
		switch m.activePage {

		case config.PageHome:
			// Home page - form handles its own keys
			// No additional key handling needed
			return m, nil

		case config.PageWallets:
			// Handle delete confirmation dialog
			if m.showDeleteDialog {
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
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("warning", fmt.Sprintf("Deleted wallet `%s`", helpers.ShortenAddr(deletedAddr)))
						m.showDeleteDialog = false
						// Load details for the newly selected wallet if split view is enabled
						return m, m.loadSelectedWalletDetails()
					}
					// Cancel deletion (No button)
					m.showDeleteDialog = false
					return m, nil
				case "esc":
					// Cancel deletion
					m.showDeleteDialog = false
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
					return m, nil
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
					return m, nil
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
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						if nickname != "" {
							m.addLog("success", fmt.Sprintf("Added wallet `%s` with nickname `%s`", helpers.ShortenAddr(newAddr), nickname))
						} else {
							m.addLog("success", fmt.Sprintf("Added wallet `%s`", helpers.ShortenAddr(newAddr)))
						}
						// Load details for the newly added wallet if split view is enabled
						return m, m.loadSelectedWalletDetails()
					} else {
						m.addError = "Invalid Etherem Address"
						m.addErrTime = time.Now()
						m.input.SetValue("")
						m.nicknameInput.SetValue("")
						m.focusedInput = 0
						m.input.Focus()
						return m, nil
					}
					return m, nil
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
				return m, nil

			case "enter":
				// Set selected wallet as active
				if len(m.accounts) > 0 {
					for i := range m.accounts {
						m.accounts[i].Active = (i == m.selectedWallet)
					}
					// Update active address to the newly activated wallet
					m.activeAddress = m.accounts[m.selectedWallet].Address
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
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
			m.activePage = config.PageSettings
			m.settingsMode = "list"
			return m, nil

		case "b", "B":
			m.activePage = config.PageDappBrowser
			m.dappMode = "list"
			return m, nil

		case "h", "H":

		case "esc":
			return m, tea.Quit

		case "delete", "backspace":
			// Show delete confirmation dialog
			if len(m.accounts) == 0 {
				return m, nil
			}
			m.showDeleteDialog = true
			m.deleteDialogYesSelected = true // Default to Yes button
			m.deleteDialogIdx = m.selectedWallet
			m.deleteDialogAddr = m.accounts[m.selectedWallet].Address
			return m, nil
		}
		return m, nil

	case config.PageDetails:
		// Don't handle keys if nicknaming form is active
		if !m.nicknaming {
			switch msg.String() {
			case "esc", "backspace":
				m.activePage = config.PageWallets
				// Load details for selected wallet if split view enabled
				return m, m.loadSelectedWalletDetails()

			case "r", "R":
				// refresh
				addr := common.HexToAddress(m.details.Address)
				m.loading = true
				m.addLog("info", fmt.Sprintf("Refreshing details for `%s`", helpers.ShortenAddr(m.details.Address)))
				return m, loadDetails(m.ethClient, addr, m.tokenWatch)

			case "n", "N":
				// nickname
				m.nicknaming = true
				m.createNicknameForm()
				return m, nil
			}
		}

	case config.PageDappBrowser:
		// Only handle list mode controls here (form handled at top of Update)
		if m.dappMode == "list" {
			switch msg.String() {
			case "esc":
				m.activePage = config.PageWallets
				// Load details for selected wallet if split view enabled
				return m, m.loadSelectedWalletDetails()

			case "enter":
				// Open Uniswap swap interface
				m.activePage = config.PageUniswap
				// Initialize Uniswap state with default values
				m.uniswapFromTokenIdx = 0 // Default to first token (ETH)
				m.uniswapToTokenIdx = 1   // Default to second token if available
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
				// Reset tracking state
				m.lastQuoteFromAmount = ""
				m.lastQuoteFromTokenIdx = -1
				m.lastQuoteToTokenIdx = -1
				return m, nil

			case "tab", "down", "right":
				// Cycle to next dApp (wraps around)
				if len(m.dapps) > 0 {
					m.selectedDappIdx = (m.selectedDappIdx + 1) % len(m.dapps)
				}
				return m, nil

			case "shift+tab", "up", "left":
				// Cycle to previous dApp (wraps around)
				if len(m.dapps) > 0 {
					m.selectedDappIdx--
					if m.selectedDappIdx < 0 {
						m.selectedDappIdx = len(m.dapps) - 1
					}
				}
				return m, nil

		case "a", "A":
			m.dappMode = "add"
			m.createAddDappForm()
			return m, nil

		case "e", "E":

		case "delete", "backspace":
			// Delete selected dApp
			if len(m.dapps) > 0 && m.selectedDappIdx < len(m.dapps) {
				deletedDapp := m.dapps[m.selectedDappIdx].Name
				m.dapps = append(m.dapps[:m.selectedDappIdx], m.dapps[m.selectedDappIdx+1:]...)
				if m.selectedDappIdx >= len(m.dapps) && m.selectedDappIdx > 0 {
					m.selectedDappIdx--
				}
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
				m.addLog("warning", fmt.Sprintf("Deleted dApp `%s`", deletedDapp))
			}
			return m, nil
			}
		}

	case config.PageSettings:
		if m.showRPCDeleteDialog {
			switch msg.String() {
			case "left", "right", "tab":
				m.deleteRPCDialogYesSelected = !m.deleteRPCDialogYesSelected
				return m, nil
			case "enter":
				if m.deleteRPCDialogYesSelected {
					idx := m.deleteRPCDialogIdx
					deletedName := m.deleteRPCDialogName
					if idx >= 0 && idx < len(m.rpcURLs) {
						m.rpcURLs = append(m.rpcURLs[:idx], m.rpcURLs[idx+1:]...)
						if m.selectedRPCIdx >= len(m.rpcURLs) && m.selectedRPCIdx > 0 {
							m.selectedRPCIdx--
						}
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("warning", fmt.Sprintf("Deleted RPC endpoint `%s`", deletedName))
					}
					m.showRPCDeleteDialog = false
					return m, nil
				}
				m.showRPCDeleteDialog = false
				return m, nil
			case "esc":
				m.showRPCDeleteDialog = false
				return m, nil
			}
			return m, nil
		}
		// Only handle list mode controls here (form handled at top of Update)
		if m.settingsMode == "list" {
			switch msg.String() {
			case "esc":
				m.activePage = config.PageWallets
				// Load details for selected wallet if split view enabled
				return m, m.loadSelectedWalletDetails()

			case "a", "A":
				m.settingsMode = "add"
				m.createAddRPCForm()
				return m, nil

			case "e", "E":
				if len(m.rpcURLs) > 0 {
					m.settingsMode = "edit"
					m.createEditRPCForm(m.selectedRPCIdx)
				}
				return m, nil

			case "delete", "backspace":
				if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
					m.showRPCDeleteDialog = true
					m.deleteRPCDialogYesSelected = true
					m.deleteRPCDialogIdx = m.selectedRPCIdx
					name := strings.TrimSpace(m.rpcURLs[m.selectedRPCIdx].Name)
					if name == "" {
						name = m.rpcURLs[m.selectedRPCIdx].URL
					}
					m.deleteRPCDialogName = name
				}
				return m, nil

			case "up", "k":
				if m.selectedRPCIdx > 0 {
					m.selectedRPCIdx--
				}
				return m, nil

			case "down", "j":
				if m.selectedRPCIdx < len(m.rpcURLs)-1 {
					m.selectedRPCIdx++
				}
				return m, nil

			case "enter", " ":
				// Set as active
				if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
					for i := range m.rpcURLs {
						m.rpcURLs[i].Active = (i == m.selectedRPCIdx)
					}
					m.rpcURL = m.rpcURLs[m.selectedRPCIdx].URL
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					// Set connecting state and reconnect with new RPC
					m.rpcConnecting = true
					m.rpcConnected = false
					return m, connectRPC(m.rpcURL)
				}
				return m, nil
			}
		}

	case config.PageUniswap:
		// Handle transaction result panel first
		if m.showTxResultPanel {
			switch msg.String() {
			case "esc", "enter":
				m.showTxResultPanel = false
				m.txResultHex = ""
				m.txResultEIP681 = ""
				m.txResultError = ""
				m.txResultPackaging = false
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
			m.activePage = config.PageDappBrowser
			return m, nil

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
					m.addLog("error", "Please enter an amount and get a quote first")
					return m, nil
				}
				if m.uniswapQuote == nil {
					m.addLog("error", "Please get a swap quote first")
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

				m.addLog("info", fmt.Sprintf("Packaging swap: %s %s → %s %s", m.uniswapFromAmount, fromToken.Symbol, m.uniswapToAmount, toToken.Symbol))
				m.showTxResultPanel = true
				m.txResultPackaging = true
				m.txResultHex = ""
				m.txResultError = ""
				m.txResultFormat = "EIP-4527"
				return m, packageSwapTransaction(m.activeAddress, fromToken, toToken, m.uniswapFromAmount, m.uniswapQuote.AmountOut, m.rpcURL)
			}
			return m, nil

		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ".":
			// Allow numeric input for amount when focused on from field
			if m.uniswapFocusedField == 0 {
				char := msg.String()
				// If not currently editing and field has a non-zero value, clear it first
				if !m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
					m.uniswapFromAmount = ""
				}
				// Prevent multiple decimal points
				if char == "." && strings.Contains(m.uniswapFromAmount, ".") {
					return m, nil
				}
				m.uniswapFromAmount += char
				m.uniswapEditingFrom = true // Mark that user is actively editing
				// Quote will be fetched when user leaves the field
				return m, nil
			} else if m.uniswapFocusedField == 1 {
				char := msg.String()
				// If not currently editing and field has a non-zero value, clear it first
				if !m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
					m.uniswapToAmount = ""
				}
				// Prevent multiple decimal points
				if char == "." && strings.Contains(m.uniswapToAmount, ".") {
					return m, nil
				}
				m.uniswapToAmount += char
				m.uniswapEditingTo = true // Mark that user is actively editing
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
						m.addLog("info", fmt.Sprintf("Max balance: %s %s", m.uniswapFromAmount, fromToken.Symbol))
						return m, m.maybeRequestUniswapQuote()
					}
				}
			}
			return m, nil
		}
	}

	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft {
			// Check for double-click on header active address
			if m.activeAddress != "" && m.headerAddrX > 0 {
				if msg.X >= m.headerAddrX && msg.X < m.headerAddrX+m.headerAddrWidth &&
					msg.Y == m.headerAddrY {
					// Check if this is a double-click (within 500ms)
					now := time.Now()
					m.addLog("debug", fmt.Sprintf("Click on header address at (%d,%d), expected (%d,%d), width=%d", msg.X, msg.Y, m.headerAddrX, m.headerAddrY, m.headerAddrWidth))
					if now.Sub(m.lastClickTime) < 500*time.Millisecond &&
						m.lastClickX == msg.X && m.lastClickY == msg.Y {
						// Double-click detected - show account list popup
						m.showAccountListPopup = true
						// Find index of current active address in accounts list
						m.accountListSelectedIdx = 0
						for i, w := range m.accounts {
							if strings.EqualFold(w.Address, m.activeAddress) {
								m.accountListSelectedIdx = i
								break
							}
						}
						m.addLog("info", "Opening account list popup")
						return m, nil
					}
					// Single click - update last click tracking
					m.lastClickTime = now
					m.lastClickX = msg.X
					m.lastClickY = msg.Y
					m.addLog("debug", fmt.Sprintf("Single click registered, waiting for double-click"))
					return m, nil
				}
			}
			// Log all clicks for debugging
			m.addLog("debug", fmt.Sprintf("Click at (%d,%d) - header check: addr='%s', X=%d, Y=%d", msg.X, msg.Y, m.activeAddress, m.headerAddrX, m.headerAddrY))
			m.addLog("debug", fmt.Sprintf("Registered %d clickable areas", len(m.clickableAreas)))

			// Check if click is on any registered clickable address
			for idx, area := range m.clickableAreas {
				if msg.X >= area.X && msg.X < area.X+area.Width &&
					msg.Y >= area.Y && msg.Y < area.Y+area.Height {
					m.addLog("debug", fmt.Sprintf("Click matched area %d: addr=%s at (%d,%d) size=%dx%d", idx, helpers.ShortenAddr(area.Address), area.X, area.Y, area.Width, area.Height))

					// Check if this is on the wallets page or popup - enable double-click activation
					if m.activePage == config.PageWallets || m.showAccountListPopup {
						now := time.Now()
						// Check if this is a double-click
						if now.Sub(m.lastClickTime) < 500*time.Millisecond &&
							m.lastClickX == msg.X && m.lastClickY == msg.Y {
							// Double-click detected on account in list - activate it
							for i, w := range m.accounts {
								if strings.EqualFold(w.Address, area.Address) {
									// Mark all as inactive
									for j := range m.accounts {
										m.accounts[j].Active = false
									}
									// Mark selected as active
									m.accounts[i].Active = true
									m.activeAddress = area.Address
									m.highlightedAddress = area.Address
									m.selectedWallet = i
									// Save config
									config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
									m.addLog("success", fmt.Sprintf("Activated account: %s", helpers.ShortenAddr(area.Address)))
									// Close popup if it was open
									if m.showAccountListPopup {
										m.showAccountListPopup = false
									}
									// Load details for newly activated wallet
									return m, m.loadSelectedWalletDetails()
								}
							}
							return m, nil
						}
						// Single click - update tracking and select the wallet
						m.lastClickTime = now
						m.lastClickX = msg.X
						m.lastClickY = msg.Y
						// Update selected wallet index for highlighting
						for i, w := range m.accounts {
							if strings.EqualFold(w.Address, area.Address) {
								m.selectedWallet = i
								m.highlightedAddress = area.Address
								if m.showAccountListPopup {
									m.accountListSelectedIdx = i
								}
								break
							}
						}
						m.addLog("debug", "Single click on account - waiting for double-click to activate")
						return m, nil
					}

					// If on details page and clicking same address, copy to clipboard
					if m.activePage == config.PageDetails && area.Address == m.details.Address {
						return m, copyToClipboard(area.Address)
					}
					// Otherwise navigate to wallet details
					// Find wallet index
					for i, w := range m.accounts {
						if strings.EqualFold(w.Address, area.Address) {
							m.selectedWallet = i
							break
						}
					}
					m.highlightedAddress = area.Address
					m.activePage = config.PageDetails
					m.loading = true
					m.details = config.WalletDetails{Address: area.Address}
					ethAddr := common.HexToAddress(area.Address)
					return m, loadDetails(m.ethClient, ethAddr, m.tokenWatch)
				}
			}

			// Handle click on transaction JSON in tx result panel
			// Make the entire panel clickable (simpler than precise coordinate tracking)
			if m.showTxResultPanel && m.txResultHex != "" {
				m.addLog("info", "Copied transaction JSON to clipboard")
				return m, copyTxJsonToClipboard(m.txResultHex)
			}

			// Legacy: handle address click on details page if no area matched
			if m.activePage == config.PageDetails && m.details.Address != "" {
				if msg.Y == m.addressLineY {
					return m, copyToClipboard(m.details.Address)
				}
			}
		}

	case clipboardCopiedMsg:
		m.copiedMsg = "✓ Copied address to clipboard"
		m.copiedMsgTime = time.Now()
		return m, clearClipboardMsg()

	case txJsonCopiedMsg:
		m.txCopiedMsg = "✓ Copied to clipboard"
		m.txCopiedMsgTime = time.Now()
		return m, clearClipboardMsg()

	case uniswapQuoteMsg:
		m.uniswapEstimating = false
		if msg.err != nil {
			m.uniswapQuoteError = msg.err.Error()
			m.uniswapQuote = nil
			m.uniswapToAmount = ""
			m.uniswapFromAmount = ""
			m.uniswapPriceImpactWarn = ""
			m.addLog("error", fmt.Sprintf("Swap quote error: %v", msg.err))
			return m, nil
		}

		m.uniswapQuoteError = ""
		m.uniswapQuote = msg.quote
		m.uniswapPriceImpactWarn = ""

		if msg.quote != nil {
			// Log detailed quote information
			tokens := m.buildTokenList()
			fromToken := tokens[m.uniswapFromTokenIdx]
			toToken := tokens[m.uniswapToTokenIdx]

			// Check if this is a reverse quote (To amount was entered, calculate From)
			isReverseQuote := m.uniswapFromAmount == "" && m.uniswapToAmount != ""

			if isReverseQuote {
				// Calculate required input amount with proper decimals
				divisorIn := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
				amountInFormatted := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountIn), divisorIn)
				m.uniswapFromAmount = amountInFormatted.Text('f', 6)
				m.uniswapEditingFrom = false

				m.addLog("info", fmt.Sprintf("📊 Reverse Quote: %s → %s", fromToken.Symbol, toToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
			} else {
				// Normal forward quote
				m.addLog("info", fmt.Sprintf("📊 Swap Quote: %s → %s", fromToken.Symbol, toToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))

				// Calculate output amount with proper decimals
				divisorOut := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
				amountOutFormatted := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountOut), divisorOut)
				m.uniswapToAmount = amountOutFormatted.Text('f', 6)

				m.addLog("info", fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
			}

			m.addLog("info", fmt.Sprintf("  Price Impact: %.4f%%", msg.quote.PriceImpact))

			// Log reserves
			divisor0 := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			reserve0Fmt := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token0Reserve), divisor0)
			reserve1Fmt := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token1Reserve), divisor0)
			m.addLog("info", fmt.Sprintf("  Reserves: %s / %s", reserve0Fmt.Text('f', 2), reserve1Fmt.Text('f', 2)))

			// Check for high price impact
			if msg.quote.PriceImpact > 1.0 {
				m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ High price impact: %.2f%%", msg.quote.PriceImpact)
				m.addLog("warn", m.uniswapPriceImpactWarn)
			} else if msg.quote.PriceImpact > 0.5 {
				m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ Moderate price impact: %.2f%%", msg.quote.PriceImpact)
			}
		}
		return m, nil

	case ensLookupResultMsg:
		m.ensLookupActive = false
		// Always log debug info
		if msg.debugInfo != "" {
			m.addLog("info", fmt.Sprintf("ENS debug: %s", msg.debugInfo))
		}
		if msg.err == nil && msg.ensName != "" && msg.address == m.ensLookupAddr {
			// Auto-populate nickname field if it's empty
			if strings.TrimSpace(m.nicknameInput.Value()) == "" {
				m.nicknameInput.SetValue(msg.ensName)
			}
			m.addLog("success", fmt.Sprintf("Found ENS name: %s", msg.ensName))
		} else if msg.err != nil && msg.address == m.ensLookupAddr {
			m.addLog("error", fmt.Sprintf("ENS lookup error: %v", msg.err))
		} else if msg.address == m.ensLookupAddr {
			m.addLog("info", "No ENS name found for address: "+helpers.FadeString(helpers.ShortenAddr(msg.address), "#F25D94", "#EDFF82"))
		}
		return m, nil

	case ensForwardResolveMsg:
		m.ensLookupActive = false
		// Always log debug info
		if msg.debugInfo != "" {
			m.addLog("info", fmt.Sprintf("ENS resolve debug: %s", msg.debugInfo))
		}
		if msg.err == nil && msg.address != "" {
			// Successfully resolved - populate address field with resolved address
			m.input.SetValue(msg.address)
			// Populate nickname field with the ENS name
			if strings.TrimSpace(m.nicknameInput.Value()) == "" {
				m.nicknameInput.SetValue(msg.ensName)
			}
			// Move to nickname field for confirmation
			m.focusedInput = 1
			m.input.Blur()
			m.nicknameInput.Focus()
			m.addLog("success", fmt.Sprintf("Resolved %s to %s", msg.ensName, helpers.ShortenAddr(msg.address)))
		} else if msg.err != nil {
			m.addLog("error", fmt.Sprintf("ENS resolution error: %v", msg.err))
			m.addError = fmt.Sprintf("Failed to resolve %s", msg.ensName)
			m.addErrTime = time.Now()
		}
		return m, nil

	default:
		// Clear clipboard message after timeout
		if msg, ok := msg.(struct{ clearClipboard bool }); ok && msg.clearClipboard {
			if time.Since(m.copiedMsgTime) >= 2*time.Second {
				m.copiedMsg = ""
			}
			if time.Since(m.txCopiedMsgTime) >= 2*time.Second {
				m.txCopiedMsg = ""
			}
		}
	}

	return m, tea.Batch(cmds...)
}
