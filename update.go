package main

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"
	uniswap "charm-wallet-tui/views/uniswap"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
	tempSendToAddr    string
	tempSendAmount    string
)

// -------------------- UPDATE --------------------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle send form updates first
	if m.activePage == config.PageWallets && m.showSendForm && m.sendForm != nil {
		return m.handleSendFormMsg(msg)
	}

	if m.activePage == config.PageHome {
		return m, m.navigateTo(config.PageWallets)
	}

	// Handle form updates first (before message switching)
	if m.activePage == config.PageDetails && m.nicknaming && m.form != nil {
		return m.handleNicknameFormMsg(msg)
	}

	// Terra Nullius claim popup handler
	if m.activePage == config.PageTerraNullius && m.activeDialog == dialogTerraClaim {
		return m.handleTerraClaimPopupMsg(msg)
	}

	if m.activePage == config.PageSettings && (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		return m.handleSettingsFormMsg(msg)
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
				log.DebugLevel: lipgloss.NewStyle().Foreground(lipgloss.Color("#874BFD")).Bold(true).SetString("DEBUG"),
				log.InfoLevel:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")).Bold(true).SetString("INFO"),
				log.WarnLevel:  lipgloss.NewStyle().Foreground(cWarn).Bold(true).SetString("WARN"),
				log.ErrorLevel: lipgloss.NewStyle().Foreground(lipgloss.Color("#F25D94")).Bold(true).SetString("ERROR"),
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
		m.contentW = helpers.Max(0, m.w-2)

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

		// V4 events viewport width: panelStyle wraps at Width-(leftPad+rightPad) = (w-2)-4 = w-6.
		// Scrollbar appends 2 chars, so viewport must be w-8 to keep content+scrollbar ≤ w-6.
		m.v4EventsViewport.Width = max(0, msg.Width-8)

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

	case terraNullClaimsCountMsg:
		m.terraNullClaimsLoading = false
		if msg.err != nil {
			m.terraNullClaimsCount = "Error"
			m.addLog("error", fmt.Sprintf("Terra Nullius number_of_claims failed: %s", msg.err.Error()))
		} else {
			m.terraNullClaimsCount = msg.count.String()
			m.addLog("success", fmt.Sprintf("Terra Nullius: %s total claims", msg.count.String()))
		}
		return m, nil

	case terraNullClaimQueryMsg:
		m.terraNullClaimQuerying = false
		if msg.err != nil {
			m.terraNullClaimResultErr = msg.err.Error()
			m.terraNullClaimResult = nil
			m.addLog("error", fmt.Sprintf("Terra Nullius claims query failed: %s", msg.err.Error()))
		} else {
			m.terraNullClaimResult = msg.result
			m.terraNullClaimResultErr = ""
			m.addLog("success", fmt.Sprintf("Terra Nullius claim #%s by %s", m.terraNullClaimInput, helpers.ShortenAddr(msg.result.Claimant)))
		}
		return m, nil

	case poolEventLineMsg:
		if m.logBuffer != nil {
			m.logBuffer.WriteString(msg.line + "\n")
			if m.logReady {
				m.updateLogViewport()
			}
		}
		if m.poolEventMonitorActive && m.poolEventMonitor != nil {
			return m, waitForPoolEvent(m.poolEventMonitor)
		}
		return m, nil

	case poolMonitorEventMsg:
		ev := msg.event
		if m.eventStore != nil {
			if err := m.eventStore.SaveV4PoolEvent(ev); err != nil {
				m.addLog("warn", fmt.Sprintf("[pool-monitor] db write error: %s", err.Error()))
			}
		}
		var cmds []tea.Cmd
		if m.poolEventMonitorActive && m.poolEventMonitor != nil {
			cmds = append(cmds, waitForPoolEventData(m.poolEventMonitor))
		}
		if m.eventStore != nil && ev.Kind == indexer.V4KindInitialize {
			cmds = append(cmds, indexERC20TokensCmd(m.eventStore, m.rpcURL, ev.Currency0, ev.Currency1))
			cmds = append(cmds, loadV4PoolTableCmd(m.eventStore))
		}
		return m, tea.Batch(cmds...)

	case v4PoolTableMsg:
		m.v4PoolRows = msg.rows
		m.v4EventsViewport.SetContent(uniswap.V4EventsContent(m.w-2, msg.rows))
		return m, nil

	case poolEventMonitorStoppedMsg:
		wasActive := m.poolEventMonitorActive
		m.poolEventMonitorActive = false
		m.poolEventMonitor = nil
		if wasActive {
			m.addLog("info", "Pool Event Monitor stopped")
		}
		return m, nil

	case v4BlockScanLineMsg:
		if m.logBuffer != nil {
			m.logBuffer.WriteString(msg.line + "\n")
			if m.logReady {
				m.updateLogViewport()
			}
		}
		if m.v4BlockScanActive && m.v4BlockScanner != nil {
			return m, waitForV4BlockScanLine(m.v4BlockScanner)
		}
		return m, nil

	case v4BlockScanDoneMsg:
		m.v4BlockScanActive = false
		m.v4BlockScanner = nil
		return m, nil

	case indexedEventMsg:
		ev := msg.event
		if m.eventStore != nil {
			if err := m.eventStore.SaveEvent(ev); err != nil {
				m.addLog("warn", fmt.Sprintf("[indexer] db write error: %s", err.Error()))
			}
		}
		m.addLog("info", "[indexer] transfer detected")
		m.logIndexedEvent(ev)
		if m.txIndexerActive && m.txIndexer != nil {
			return m, waitForIndexedEvent(m.txIndexer)
		}
		return m, nil

	case indexerStoppedMsg:
		wasActive := m.txIndexerActive
		m.txIndexerActive = false
		m.txIndexer = nil
		if wasActive {
			m.addLog("info", "Address indexer stopped")
		}
		return m, nil

	case erc20TokenIndexedMsg:
		return m, nil

	case v4PoolEventMsg:
		ev := msg.event
		if m.eventStore != nil {
			if err := m.eventStore.SaveV4PoolEvent(ev); err != nil {
				m.addLog("warn", fmt.Sprintf("[v4-indexer] db write error: %s", err.Error()))
			}
		}
		m.logV4PoolEvent(ev)
		var cmds []tea.Cmd
		if m.txIndexerActive && m.txIndexer != nil {
			cmds = append(cmds, waitForV4PoolEvent(m.txIndexer))
		}
		if m.eventStore != nil && ev.Kind == indexer.V4KindInitialize {
			cmds = append(cmds, indexERC20TokensCmd(m.eventStore, m.rpcURL, ev.Currency0, ev.Currency1))
		}
		return m, tea.Batch(cmds...)

	case v4PoolIndexerStoppedMsg:
		return m, nil

	case indexerProgressMsg:
		count := int64(0)
		if m.eventStore != nil {
			count, _ = m.eventStore.Count()
		}
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#7EE787")).Bold(true).Render("[INDEXER]")
		m.addLog("info", fmt.Sprintf("%s backscan block=%d  indexed=%d records", prefix, msg.block, count))
		if m.txIndexerActive && m.txIndexer != nil {
			return m, waitForIndexerProgress(m.txIndexer)
		}
		return m, nil

	case recentEventsMsg:
		if msg.err != nil {
			m.addLog("warn", fmt.Sprintf("[indexer] failed to load history: %s", msg.err.Error()))
			return m, nil
		}
		m.addLog("info", fmt.Sprintf("[indexer] %d events in store — showing last %d", msg.count, len(msg.events)))
		// Log events in chronological order (they arrive newest-first from DB)
		for i := len(msg.events) - 1; i >= 0; i-- {
			m.addLog("info", fmt.Sprintf("[history] event %d of %d", len(msg.events)-i, len(msg.events)))
			m.logIndexedEvent(msg.events[i])
		}
		return m, nil

	case poolInfoResultMsg:
		m.poolInfoLoading = false
		m.poolInfoID = msg.poolID
		shortID := msg.poolID
		if len(shortID) > 16 {
			shortID = shortID[:10] + "…" + shortID[len(shortID)-6:]
		}
		if msg.err != nil {
			m.poolInfoErr = msg.err.Error()
			m.poolInfoData = nil
			m.addLog("error", fmt.Sprintf("Pool Info: failed for pool %s: %s", shortID, msg.err.Error()))
			return m, nil
		}
		m.poolInfoData = msg.info
		m.poolInfoErr = ""
		m.addLog("success", fmt.Sprintf("Pool Info: pool %s — sqrtPrice=%s tick=%d liquidity=%s", shortID, msg.info.SqrtPriceX96, msg.info.Tick, msg.info.Liquidity))
		// Chain the eth_getLogs fetch for pool key (currency0/1, fee, etc.)
		m.poolInfoKeyLoading = true
		m.poolInfoKeyErr = ""
		m.addLog("info", fmt.Sprintf("Pool Info: fetching Initialize event log for pool %s", shortID))
		return m, fetchPoolKey(m.rpcURL, msg.poolID)

	case poolKeyResultMsg:
		m.poolInfoKeyLoading = false
		shortID := msg.poolID
		if len(shortID) > 16 {
			shortID = shortID[:10] + "…" + shortID[len(shortID)-6:]
		}
		if msg.err != nil {
			m.poolInfoKeyErr = msg.err.Error()
			m.addLog("warn", fmt.Sprintf("Pool Info: Initialize event lookup failed for pool %s: %s", shortID, msg.err.Error()))
			return m, nil
		}
		if m.poolInfoData != nil {
			m.poolInfoData.Currency0 = msg.key.Currency0
			m.poolInfoData.Currency1 = msg.key.Currency1
			m.poolInfoData.Fee = msg.key.Fee
			m.poolInfoData.TickSpacing = msg.key.TickSpacing
			m.poolInfoData.Hooks = msg.key.Hooks
		}
		m.poolInfoKeyErr = ""
		m.addLog("success", fmt.Sprintf("Pool Info: pool key for %s — currency0=%s currency1=%s fee=%d", shortID, msg.key.Currency0, msg.key.Currency1, msg.key.Fee))
		return m, nil

	case tea.KeyMsg:
		// Handle pool info popup (closes on Enter or Esc regardless of active page)
		if m.activeDialog == dialogPoolInfo {
			switch msg.String() {
			case "enter", "esc":
				m.activeDialog = dialogNone
				m.poolInfoData = nil
				m.poolInfoErr = ""
				m.poolInfoID = ""
				m.poolInfoKeyLoading = false
				m.poolInfoKeyErr = ""
			}
			return m, nil
		}

		// Handle transaction result panel FIRST (before any other keys)
		if m.activeDialog == dialogTxResult {
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
				m.activeDialog = dialogNone
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
		if m.activeDialog == dialogAccountList {
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
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
					m.addLog("success", fmt.Sprintf("Activated account: %s", helpers.ShortenAddr(selectedAddr)))
					// Close popup
					m.activeDialog = dialogNone
					// Load details for newly activated wallet
					return m, m.loadSelectedWalletDetails()
				}
				return m, nil
			case "esc":
				m.activeDialog = dialogNone
				return m, nil
			}
			return m, nil
		}

		allowMenuHotkeys := !m.textInputActive()
		// global keys
		if allowMenuHotkeys {
			switch msg.String() {
			case "ctrl+c":
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
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
					return m, tea.Batch(initLogViewport(), m.logSpinner.Tick)
				}
				// Clear logs and de-initialize when disabling
				if m.logBuffer != nil {
					m.logBuffer.Reset()
				}
				m.logger = nil
				m.logReady = false
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
				return m, nil

			case "i", "I":
				if m.txIndexerActive {
					if m.txIndexer != nil {
						m.txIndexer.Stop()
					}
					m.txIndexerActive = false
					m.addLog("info", "Address indexer stopped")
					return m, nil
				}
				if !m.rpcConnected || m.ethClient == nil {
					m.addLog("warn", "Indexer requires an active RPC connection")
					return m, nil
				}
				if len(m.accounts) == 0 {
					m.addLog("warn", "No saved addresses to index")
					return m, nil
				}
				addrs := make([]common.Address, len(m.accounts))
				for i, a := range m.accounts {
					addrs[i] = common.HexToAddress(a.Address)
				}
				m.txIndexer = indexer.New()
				m.txIndexer.Start(m.rpcURL, addrs, m.tokenWatch)
				m.txIndexerActive = true
				addrLabels := make([]string, len(addrs))
			for i, a := range addrs {
				addrLabels[i] = helpers.HyperAddr(a)
			}
			m.addLog("info", fmt.Sprintf("Address indexer started — scanning backward from current block, watching: %s", strings.Join(addrLabels, "  ")))
				startCmds := []tea.Cmd{waitForIndexedEvent(m.txIndexer), waitForV4PoolEvent(m.txIndexer), waitForIndexerProgress(m.txIndexer)}
				if m.eventStore != nil {
					startCmds = append(startCmds, loadRecentEvents(m.eventStore, 50))
				} else if m.eventStoreErr != "" {
					m.addLog("warn", fmt.Sprintf("[indexer] event store unavailable: %s", m.eventStoreErr))
				}
				return m, tea.Batch(startCmds...)

			case "pageup", "pagedown", "up", "down":
				v4Visible := m.activePage == config.PageUniswap && m.poolEventMonitorActive && !m.uniswapShowingLiquidity
				bothVisible := v4Visible && m.logEnabled && m.logReady
				var cmd tea.Cmd
				switch {
				case bothVisible && m.focusedPanel == focusedPanelLog:
					m.logViewport, cmd = m.logViewport.Update(msg)
				case v4Visible:
					m.v4EventsViewport, cmd = m.v4EventsViewport.Update(msg)
				case m.logEnabled && m.logReady:
					m.logViewport, cmd = m.logViewport.Update(msg)
				}
				if cmd != nil {
					return m, cmd
				}
			}
		}

		// page-specific behavior
		switch m.activePage {
		case config.PageHome:
			return m, nil
		case config.PageWallets:
			return m.handleWalletsKey(msg)
		case config.PageDetails:
			return m.handleDetailsKey(msg)
		case config.PageDappBrowser:
			return m.handleDappsKey(msg)
		case config.PageSettings:
			return m.handleSettingsKey(msg)
		case config.PageUniswap:
			return m.handleUniswapKey(msg)
		case config.PageTerraNullius:
			return m.handleTerraKey(msg)
		}

	case tea.MouseMsg:
		// Consume all clicks while Pool Info popup is showing.
		if m.activeDialog == dialogPoolInfo {
			if msg.Type == tea.MouseLeft {
				switch {
				case msg.Y == m.poolInfoOKBtnY &&
					msg.X >= m.poolInfoOKBtnX1 && msg.X < m.poolInfoOKBtnX2:
					m.activeDialog = dialogNone
					m.poolInfoData = nil
					m.poolInfoErr = ""
					m.poolInfoID = ""
					m.poolInfoCopied = false
					m.poolInfoKeyLoading = false
					m.poolInfoKeyErr = ""
				case msg.Y == m.poolInfoIDLineY &&
					msg.X >= m.poolInfoIDLineX1 && msg.X < m.poolInfoIDLineX2:
					return m, copyPoolIDToClipboard(m.poolInfoID)
				}
			}
			return m, nil
		}

		// Route mouse wheel events to the focused panel.
		if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
			v4Visible := m.activePage == config.PageUniswap && m.poolEventMonitorActive && !m.uniswapShowingLiquidity
			bothVisible := v4Visible && m.logEnabled && m.logReady
			var cmd tea.Cmd
			switch {
			case bothVisible && m.focusedPanel == focusedPanelLog:
				m.logViewport, cmd = m.logViewport.Update(msg)
			case v4Visible:
				m.v4EventsViewport, cmd = m.v4EventsViewport.Update(msg)
			case m.logEnabled && m.logReady:
				m.logViewport, cmd = m.logViewport.Update(msg)
			}
			if cmd != nil {
				return m, cmd
			}
		}

		// Scrollbar drag: release ends drag regardless of position.
		if msg.Type == tea.MouseRelease {
			m.logScrollDragging = false
			return m, nil
		}

		// Scrollbar drag: motion while dragging updates scroll offset.
		if msg.Type == tea.MouseMotion && m.logScrollDragging {
			m.applyScrollbarDrag(msg.Y)
			return m, nil
		}

		if msg.Type == tea.MouseLeft {
			// Panel focus: when both V4 events and log panels are visible, set focus
			// based on which region was clicked. Does not consume the click.
			if m.activePage == config.PageUniswap && m.poolEventMonitorActive &&
				!m.uniswapShowingLiquidity && m.logEnabled && m.logPanelTop > 3 {
				if msg.Y >= m.logPanelTop-3 {
					m.focusedPanel = focusedPanelLog
				} else {
					m.focusedPanel = focusedPanelV4Events
				}
			}

			// Scrollbar click: detect click near the scrollbar column and start dragging.
			// ±1 tolerance accounts for terminal/lipgloss coordinate rounding.
			if m.logEnabled && m.logReady && m.logPanelTop > 0 {
				scrollbarX := m.logViewport.Width + 3
				vpBottom := m.logPanelTop + m.logViewport.Height - 1
				if msg.X >= scrollbarX-1 && msg.X <= scrollbarX+1 &&
					msg.Y >= m.logPanelTop && msg.Y <= vpBottom {
					m.logScrollDragging = true
					m.applyScrollbarDrag(msg.Y)
					return m, nil
				}
			}

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
						m.activeDialog = dialogAccountList
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
			// If the click lands inside the log viewport content rows, resolve which
			// OSC 8 hyperlink (if any) was clicked and open it in the browser.
			// Log panel content starts at X=2 (1 border + 1 padding from logview.Render).
			if m.logEnabled && m.logReady && m.logPanelTop > 0 && m.logBuffer != nil {
				lines := strings.Split(m.logBuffer.String(), "\n")
				viewportLine := msg.Y - m.logPanelTop
				absoluteLine := viewportLine + m.logViewport.YOffset
				if viewportLine >= 0 && absoluteLine < len(lines) {
					lineCol := msg.X - 2 // subtract border(1) + padding(1)
					if url := urlAtCol(lines[absoluteLine], lineCol); url != "" {
						if strings.HasPrefix(url, "poolinfo://") {
							poolIDHex := strings.TrimPrefix(url, "poolinfo://")
							m.activeDialog = dialogPoolInfo
							m.poolInfoLoading = true
							m.poolInfoID = poolIDHex
							m.poolInfoData = nil
							m.poolInfoErr = ""
							shortID := poolIDHex
							if len(shortID) > 16 {
								shortID = shortID[:10] + "…" + shortID[len(shortID)-6:]
							}
							m.addLog("info", fmt.Sprintf("Pool Info: querying pool %s", shortID))
							return m, fetchPoolInfo(m.rpcURL, poolIDHex)
						}
						return m, openInBrowser(url)
					}
				}
			}

			// V4 events panel click: detect OSC 8 poolinfo:// hyperlinks in viewport content.
			// panelStyle X offset: 1 border + 2 padding = 3.  Viewport starts at m.v4ViewportTop.
			if m.activePage == config.PageUniswap && m.poolEventMonitorActive &&
				!m.uniswapShowingLiquidity && m.v4ViewportTop > 0 {
				vpHeight := m.v4EventsViewport.Height
				if msg.Y >= m.v4ViewportTop && msg.Y < m.v4ViewportTop+vpHeight {
					viewportLine := msg.Y - m.v4ViewportTop
					absoluteLine := viewportLine + m.v4EventsViewport.YOffset
					rawContent := uniswap.V4EventsContent(m.w-2, m.v4PoolRows)
					lines := strings.Split(rawContent, "\n")
					if absoluteLine < len(lines) {
						lineCol := msg.X - 3 // subtract panelStyle: 1 border + 2 padding
						if url := urlAtCol(lines[absoluteLine], lineCol); url != "" {
							if strings.HasPrefix(url, "poolinfo://") {
								poolIDHex := strings.TrimPrefix(url, "poolinfo://")
								m.activeDialog = dialogPoolInfo
								m.poolInfoLoading = true
								m.poolInfoID = poolIDHex
								m.poolInfoData = nil
								m.poolInfoErr = ""
								shortID := poolIDHex
								if len(shortID) > 16 {
									shortID = shortID[:10] + "…" + shortID[len(shortID)-6:]
								}
								m.addLog("info", fmt.Sprintf("Pool Info: querying pool %s", shortID))
								return m, fetchPoolInfo(m.rpcURL, poolIDHex)
							}
							return m, openInBrowser(url)
						}
					}
				}
			}

			// Log all clicks for debugging (suppressed during scrollbar drag)
			if !m.logScrollDragging {
				m.addLog("debug", fmt.Sprintf("Click at (%d,%d) - header check: addr='%s', X=%d, Y=%d", msg.X, msg.Y, m.activeAddress, m.headerAddrX, m.headerAddrY))
				m.addLog("debug", fmt.Sprintf("Registered %d clickable areas", len(m.clickableAreas)))
			}

			// Check if click is on any registered clickable address
			for idx, area := range m.clickableAreas {
				if msg.X >= area.X && msg.X < area.X+area.Width &&
					msg.Y >= area.Y && msg.Y < area.Y+area.Height {
					m.addLog("debug", fmt.Sprintf("Click matched area %d: addr=%s at (%d,%d) size=%dx%d", idx, helpers.ShortenAddr(area.Address), area.X, area.Y, area.Width, area.Height))

					// Check if this is on the wallets page or popup - enable double-click activation
					if m.activePage == config.PageWallets || m.activeDialog == dialogAccountList {
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
									config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
									m.addLog("success", fmt.Sprintf("Activated account: %s", helpers.ShortenAddr(area.Address)))
									// Close popup if it was open
									if m.activeDialog == dialogAccountList {
										m.activeDialog = dialogNone
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
								if m.activeDialog == dialogAccountList {
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
			if m.activeDialog == dialogTxResult && m.txResultHex != "" {
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

	case poolIDCopiedMsg:
		m.poolInfoCopied = true
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return struct{ clearPoolIDCopied bool }{true}
		})

	case struct{ clearPoolIDCopied bool }:
		m.poolInfoCopied = false
		return m, nil

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

	case liquidityPositionsMsg:
		m.liquidityLoading = false
		if msg.err != nil {
			m.liquidityErr = msg.err.Error()
			m.addLog("error", "Liquidity positions error: "+msg.err.Error())
			for _, d := range msg.diagnostics {
				m.addLog("info", "  "+d)
			}
		} else {
			m.liquidityPositions = msg.positions
			m.addLog("info", fmt.Sprintf("V4 PositionManager: balanceOf=%d NFT(s)", msg.nftCount))
			for _, d := range msg.diagnostics {
				m.addLog("info", "  "+d)
			}
			if len(msg.positions) == 0 {
				m.addLog("info", "No active V4 liquidity positions found")
			} else {
				m.addLog("info", fmt.Sprintf("%d position(s) with active liquidity", len(msg.positions)))
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

// appendNumericInput appends a digit or '.' to value.
// On first keystroke after a non-zero value, the field is cleared.
// Returns (newValue, newEditing, ok) — ok is false when a duplicate '.' should be dropped.
func appendNumericInput(value string, editing bool, char string) (string, bool, bool) {
	if !editing && value != "" && value != "0" {
		value = ""
	}
	if char == "." && strings.Contains(value, ".") {
		return value, editing, false
	}
	return value + char, true, true
}
