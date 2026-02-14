package main

import (
	"fmt"
	"image/color"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/dapps"
	"charm-wallet-tui/views/details"
	logview "charm-wallet-tui/views/log"
	"charm-wallet-tui/views/settings"
	"charm-wallet-tui/views/uniswap"
	"charm-wallet-tui/views/wallets"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// -------------------- VIEW --------------------

func (m model) renderDeleteDialog() string {
	var (
		dialogBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(1, 0).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(true)

		buttonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#888B7E")).
				Padding(0, 3).
				MarginTop(1)

		activeButtonStyle = buttonStyle.Copy().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#F25D94")).
				MarginRight(2).
				Underline(true)
	)
	msg := helpers.FadeString("Are you sure you want to delete the account "+helpers.ShortenAddr(m.deleteDialogAddr)+"?", "#F25D94", "#EDFF82")
	question := lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(msg)

	// Apply active style to the selected button
	var okButton, cancelButton string
	if m.deleteDialogYesSelected {
		okButton = activeButtonStyle.Render("Yes")
		cancelButton = buttonStyle.Render("No")
	} else {
		okButton = buttonStyle.Copy().MarginRight(2).Render("Yes")
		cancelButton = activeButtonStyle.Copy().MarginRight(0).Render("No")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, okButton, cancelButton)
	ui := lipgloss.JoinVertical(lipgloss.Center, question, buttons)

	dialog := dialogBoxStyle.Render(ui)

	// Center the dialog on screen
	return lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m model) renderRPCDeleteDialog() string {
	var (
		dialogBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(1, 0).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(true)

		buttonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#888B7E")).
				Padding(0, 3).
				MarginTop(1)

		activeButtonStyle = buttonStyle.Copy().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#F25D94")).
				MarginRight(2).
				Underline(true)
	)
	msg := helpers.FadeString("Are you sure you want to delete the RPC endpoint "+m.deleteRPCDialogName+"?", "#F25D94", "#EDFF82")
	question := lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(msg)

	var okButton, cancelButton string
	if m.deleteRPCDialogYesSelected {
		okButton = activeButtonStyle.Render("Yes")
		cancelButton = buttonStyle.Render("No")
	} else {
		okButton = buttonStyle.Copy().MarginRight(2).Render("Yes")
		cancelButton = activeButtonStyle.Copy().MarginRight(0).Render("No")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, okButton, cancelButton)
	ui := lipgloss.JoinVertical(lipgloss.Center, question, buttons)

	dialog := dialogBoxStyle.Render(ui)

	return lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m *model) renderAccountListPopup() string {
	var (
		dialogBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(1, 2).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(true).
				Background(cPanel)
	)

	title := lipgloss.NewStyle().
		Foreground(cAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(70).
		Render("Select Account")

	// Use the wallets view to render the account list
	accountList, _, _ := wallets.RenderList(m.accounts, m.accountListSelectedIdx)

	help := lipgloss.NewStyle().
		Foreground(cMuted).
		Align(lipgloss.Center).
		Width(70).
		MarginTop(1).
		Render("↑/↓: Navigate • Enter: Select • Esc: Cancel")

	ui := lipgloss.JoinVertical(lipgloss.Left, title, "", accountList, help)

	dialog := dialogBoxStyle.Render(ui)

	// Center the dialog on screen
	return lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m *model) renderTxResultContent() string {
	format := m.txResultFormat
	if format == "" {
		format = "EIP-4527"
	}

	label := "EIP-4527 Transaction JSON:"
	if format == "EIP-681" {
		label = "EIP-681 Transaction URL:"
	}

	txResultContent := styles.TitleStyle.Render("Transaction Ready To Sign ("+format+")") + "\n\n"

	if m.txResultPackaging {
		txResultContent += m.spin.View() + " Packaging transaction..."
	} else if m.txResultError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
		txResultContent += errorStyle.Render("Error: " + m.txResultError)
		txResultContent += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Press ESC or Enter to close")
	} else {
		qrCode := rpc.GenerateQRCode(m.txResultEIP681)
		qrStyle := lipgloss.NewStyle()
		txResultContent += qrStyle.Render(qrCode) + "\n"

		txResultContent += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(label) + "\n\n"
		txResultContent += m.txResultHex

		txResultContent += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Scan the QR code with your wallet app to sign this transaction")
		txResultContent += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Click anywhere or press Ctrl+C to copy • Press ESC or Enter to close")

		// Show copied message if present
		if m.txCopiedMsg != "" {
			txResultContent += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(m.txCopiedMsg)
		}
	}
	return txResultContent
}

func (m *model) renderTxResultPanel() string {
	contentWidth := max(0, m.w-8)
	centeredContent := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(m.renderTxResultContent())
	content := panelStyle.Width(max(0, m.w-4)).Render(centeredContent)
	return appStyle.Render(lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		content,
	))
}

func (m *model) globalHeader() string {
	availableWidth := max(0, m.w-8) // Account for panel padding

	// Active Address (the one marked with ★)
	var addrDisplay string
	if m.activeAddress != "" {
		addrDisplay = lipgloss.NewStyle().
			Foreground(cAccent2).
			Bold(true).
			Render("Active Address: " + helpers.FadeString(helpers.ShortenAddr(m.activeAddress), "#F25D94", "#EDFF82"))

		// Track header address position for double-click detection
		// X position: 3 (panel left border + padding) + length of "Active Address: "
		// Y position: 2 (accounting for panel top border + padding = row 1, but 0-indexed)
		m.headerAddrX = 3 + len("Active Address: ")
		m.headerAddrY = 2
		m.headerAddrWidth = len(helpers.ShortenAddr(m.activeAddress))
	} else {
		addrDisplay = lipgloss.NewStyle().
			Foreground(cMuted).
			Render("Active Address: No selection")
		m.headerAddrX = 0
		m.headerAddrY = 0
		m.headerAddrWidth = 0
	}

	// RPC Status with green dot
	var statusIcon string
	var statusColor lipgloss.Color
	var statusText string

	if m.rpcURL == "" {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "No RPC"
	} else if m.rpcConnecting {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "Connecting..."
	} else if !m.rpcConnected {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "Connection Failed"
	} else {
		statusIcon = "●"
		statusColor = cAccent
		// Find active RPC name
		for _, r := range m.rpcURLs {
			if r.Active && r.URL == m.rpcURL {
				statusText = r.Name
				break
			}
		}
		if statusText == "" {
			statusText = "Connected"
		}
	}

	rpcDisplay := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true).
		Render(statusIcon + " " + statusText)

	// Center title
	titleStyle := lipgloss.NewStyle().
		Foreground(cAccent).
		Bold(true)
	titleText := titleStyle.Render(helpers.FadeString("domestic system", "#7EE787", "#82CFFD"))

	// Calculate widths
	addrWidth := lipgloss.Width(addrDisplay)
	rpcWidth := lipgloss.Width(rpcDisplay)
	titleWidth := lipgloss.Width(titleText)

	// Calculate spacing to center the title
	totalOtherWidth := addrWidth + rpcWidth + titleWidth

	var headerLine string
	if totalOtherWidth+4 > availableWidth {
		// Not enough space, stack vertically
		headerLine = addrDisplay + "\n" + titleText + "\n" + rpcDisplay
	} else {
		// Three-column layout: Address | Title (centered) | RPC
		// Calculate how much space for padding
		remainingSpace := availableWidth - totalOtherWidth
		leftPadding := remainingSpace / 2
		rightPadding := remainingSpace - leftPadding

		leftSpacer := strings.Repeat(" ", max(1, leftPadding))
		rightSpacer := strings.Repeat(" ", max(1, rightPadding))

		headerLine = addrDisplay + leftSpacer + titleText + rightSpacer + rpcDisplay
	}

	// Add separator line
	separator := lipgloss.NewStyle().
		Foreground(cBorder).
		Render(strings.Repeat("─", availableWidth))

	return headerLine + "\n" + separator
}

func (m *model) View() string {
	// Clear clickable areas for fresh render
	m.clickableAreas = nil

	// Render global header outside of page content
	globalHdr := m.globalHeader()
	headerPanel := panelStyle.Width(max(0, m.w-2)).Render(globalHdr)

	// Note: Header address clickable area coordinates are set in globalHeader()

	var pageContent string
	var nav string

	switch m.activePage {
	case config.PageHome:
		// TODO: home view not implemented yet
		pageContent = panelStyle.Width(max(0, m.w-2)).Render("Home view not implemented")
		nav = ""

	case config.PageWallets:
		walletsContent, walletsClickableAreas := wallets.Render(m.accounts, m.selectedWallet, m.addError)

		// Register clickable areas from wallets view
		// Adjust Y coordinates to account for header and panel borders
		for _, area := range walletsClickableAreas {
			m.clickableAreas = append(m.clickableAreas, config.ClickableArea{
				X:       area.X,
				Y:       area.Y + 1, // Minimal offset for panel border
				Width:   area.Width,
				Height:  area.Height,
				Address: area.Address,
			})
		}

		// Show add wallet form if in adding mode
		if m.adding {
			inputView := m.input.View() + "\n" + m.nicknameInput.View() + "\n"

			// Show ENS lookup status
			if m.ensLookupActive {
				inputView += m.spin.View() + " ENS lookup…\n"
			}

			inputView += hotkeyStyle.Render("Tab") + " next field   " +
				hotkeyStyle.Render("Enter") + " next/save   " +
				hotkeyStyle.Render("Esc") + " cancel   " +
				hotkeyStyle.Render("Ctrl+v") + " paste"

			// Show error message if present and recent
			if m.addError != "" && time.Since(m.addErrTime) < 3*time.Second {
				errorStyle := lipgloss.NewStyle().Foreground(cWarn).Bold(true)
				inputView += "\n" + errorStyle.Render(m.addError)
			}

			addBoxView := "\n\n" + panelStyle.
				BorderForeground(cAccent2).
				Render(inputView)
			walletsContent += addBoxView
		}

		// If detailsInWallets is enabled and we have a selected wallet, show split view
		if m.detailsInWallets && len(m.accounts) > 0 {
			// Convert local config.WalletDetails to rpc.WalletDetails
			rpcDetails := rpc.WalletDetails{
				Address:    m.details.Address,
				EthWei:     m.details.EthWei,
				LoadedAt:   m.details.LoadedAt,
				ErrMessage: m.details.ErrMessage,
			}
			for _, t := range m.details.Tokens {
				rpcDetails.Tokens = append(rpcDetails.Tokens, rpc.TokenBalance{
					Symbol:   t.Symbol,
					Decimals: t.Decimals,
					Balance:  t.Balance,
				})
			}

			detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())

			// Show transaction result panel if active
			if m.showTxResultPanel {
				detailsContent = m.renderTxResultContent()
				// Show send form if active
			} else if m.showSendForm && m.sendForm != nil {
				sendFormContent := styles.TitleStyle.Render("Send Transaction") + "\n\n" + m.sendForm.View()
				detailsContent = sendFormContent
			} else if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
				// Add send button if ETH balance > 0 and form is not active
				var sendButtonStyle lipgloss.Style
				if m.sendButtonFocused {
					sendButtonStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#FFF7DB")).
						Background(lipgloss.Color("#F25D94")).
						Padding(0, 3).
						MarginTop(2).
						Underline(true)
				} else {
					sendButtonStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#FFF7DB")).
						Background(lipgloss.Color("#888B7E")).
						Padding(0, 3).
						MarginTop(2)
				}
				sendButton := sendButtonStyle.Render("Send")
				detailsContent += "\n\n" + sendButton

				// Add hint text
				if !m.sendButtonFocused {
					hintText := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#666666")).
						MarginTop(1).
						Render("Press Tab to select")
					detailsContent += "\n" + hintText
				} else {
					hintText := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#666666")).
						MarginTop(1).
						Render("Press Enter to send")
					detailsContent += "\n" + hintText
				}
			}

			// Calculate panel widths (split 40/60)
			listWidth := max(0, (m.w*4)/10-2)
			detailsWidth := max(0, (m.w*6)/10-2)

			// Get the height of the left panel content to match it on the right
			leftPanel := panelStyle.Width(listWidth).Render(walletsContent)
			leftPanelHeight := lipgloss.Height(leftPanel)

			// Set the right panel to match the left panel height
			rightPanel := panelStyle.
				Width(detailsWidth + 1).
				Height(leftPanelHeight - 2).
				Render(detailsContent)

			pageContent = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
		} else {
			pageContent = panelStyle.Width(max(0, m.w-2)).Render(walletsContent)
		}
		nav = wallets.Nav(m.w - 2)

		// Render delete confirmation dialog overlay
		if m.showDeleteDialog {
			// Dialog overlays the current view
			return m.renderDeleteDialog()
		}

		if m.showTxResultPanel {
			return m.renderTxResultPanel()
		}

	case config.PageDappBrowser:
		dappBrowserContent := dapps.Render(m.dapps, m.selectedDappIdx)

		// Show form if in add/edit mode
		if (m.dappMode == "add" || m.dappMode == "edit") && m.form != nil {
			dappBrowserContent = styles.TitleStyle.Render("dApp Browser") + "\n\n" + m.form.View()
		}

		pageContent = panelStyle.Width(max(0, m.w-2)).Render(dappBrowserContent)
		nav = dapps.Nav(m.w-2, m.dappMode)

	case config.PageDetails:
		// Convert local config.WalletDetails to rpc.WalletDetails
		rpcDetails := rpc.WalletDetails{
			Address:    m.details.Address,
			EthWei:     m.details.EthWei,
			LoadedAt:   m.details.LoadedAt,
			ErrMessage: m.details.ErrMessage,
		}
		for _, t := range m.details.Tokens {
			rpcDetails.Tokens = append(rpcDetails.Tokens, rpc.TokenBalance{
				Symbol:   t.Symbol,
				Decimals: t.Decimals,
				Balance:  t.Balance,
			})
		}

		detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())
		pageContent = panelStyle.Width(max(0, m.w-2)).Render(detailsContent)
		nav = details.Nav(m.w-2, m.nicknaming)

	case config.PageSettings:
		settingsContent := settings.Render(m.rpcURLs, m.selectedRPCIdx)

		// Show form if in add/edit mode
		if (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
			settingsContent = styles.TitleStyle.Render("RPC Settings") + "\n\n" + m.form.View()
		}

		pageContent = panelStyle.Width(max(0, m.w-2)).Render(settingsContent)
		nav = settings.Nav(m.w-2, m.settingsMode)

		if m.showRPCDeleteDialog {
			return m.renderRPCDeleteDialog()
		}

	case config.PageUniswap:
		// Build token list from wallet details
		tokens := m.buildTokenList()

		// If showing token selector, render popup
		if m.uniswapShowingSelector {
			uniswapView := uniswap.RenderTokenSelector(
				m.w,
				m.h-8, // Account for header and nav
				tokens,
				m.uniswapSelectorIdx,
				m.uniswapSelectorFor == 0,
			)
			pageContent = uniswapView
			nav = uniswap.Nav(m.w - 2)
		} else {
			// Render main swap interface
			uniswapView := uniswap.Render(
				m.w-2,
				m.h-8, // Account for header and nav
				tokens,
				m.uniswapFromTokenIdx,
				m.uniswapToTokenIdx,
				m.uniswapFromAmount,
				m.uniswapToAmount,
				m.uniswapFocusedField,
				m.uniswapEstimating,
				m.uniswapPriceImpactWarn,
			)
			// Wrap in panel style to constrain properly
			pageContent = panelStyle.Width(max(0, m.w-2)).Render(uniswapView)
			nav = uniswap.Nav(m.w - 2)
		}

		// Show transaction result panel overlay if active
		if m.showTxResultPanel {
			return m.renderTxResultPanel()
		}
	}

	// Render log panel only if enabled
	var logPanel string
	if m.logEnabled {
		// Ensure viewport height stays in sync with the rendered panel
		reservedHeight := 10
		availableHeight := max(5, m.h-reservedHeight)
		maxLogHeight := min(m.h/3, 15)
		logPanelHeight := min(availableHeight, maxLogHeight)
		m.logViewport.Height = logPanelHeight

		logPanel = logview.Render(m.w, m.h, m.logReady, m.logSpinner.View(), m.logViewport)
		content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav, logPanel)
		baseView := appStyle.Render(content)

		// Show account list popup overlay if active
		if m.showAccountListPopup {
			popup := m.renderAccountListPopup()
			return popup // Popup uses lipgloss.Place internally, so just return it
		}

		return baseView
	}

	// Use lipgloss to join sections vertically (without log panel)
	content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav)
	baseView := appStyle.Render(content)

	// Show account list popup overlay if active
	if m.showAccountListPopup {
		// Register clickable areas for popup before rendering
		accountList, popupClickableAreas, _ := wallets.RenderList(m.accounts, m.accountListSelectedIdx)

		// Calculate popup position (centered)
		dialogHeight := len(m.accounts)*3 + 8 // Approximate dialog height
		dialogWidth := 74 // Width including border and padding
		popupStartX := (m.w - dialogWidth) / 2
		popupStartY := (m.h - dialogHeight) / 2

		// Clear and register clickable areas for popup
		m.clickableAreas = nil

		// RenderList returns areas with Y starting at 9 (for main panel)
		// and X starting at 4 (for indentation)
		// In popup we need to:
		// - Subtract the base offsets from RenderList (9 for Y, 4 for X)
		// - Add popup position + border (1) + padding (1 top, 2 left) + title lines (2)
		const renderListBaseY = 9
		const renderListBaseX = 4
		popupContentStartX := popupStartX + 1 + 2 // border + padding left
		popupContentStartY := popupStartY + 1 + 1 + 2 // border + padding top + title + blank line

		m.addLog("debug", fmt.Sprintf("Popup position: (%d,%d), content starts at: (%d,%d)", popupStartX, popupStartY, popupContentStartX, popupContentStartY))

		for i, area := range popupClickableAreas {
			adjustedX := popupContentStartX + (area.X - renderListBaseX)
			adjustedY := popupContentStartY + (area.Y - renderListBaseY)
			m.clickableAreas = append(m.clickableAreas, config.ClickableArea{
				X:       adjustedX,
				Y:       adjustedY,
				Width:   area.Width,
				Height:  area.Height,
				Address: area.Address,
			})
			if i == 0 {
				m.addLog("debug", fmt.Sprintf("First area: orig=(%d,%d) adjusted=(%d,%d) addr=%s", area.X, area.Y, adjustedX, adjustedY, helpers.ShortenAddr(area.Address)))
			}
		}

		// Suppress unused variable warning
		_ = accountList

		popup := m.renderAccountListPopup()
		return popup // Popup uses lipgloss.Place internally, so just return it
	}

	return baseView
}

func key(s string) string {
	return hotkeyKeyStyle.Render(s)
}

func rpcStatus(url string, c *rpc.Client) string {
	if url == "" {
		return "not set"
	}
	if c == nil {
		return "connecting/failed"
	}
	return "connected"
}

func rainbow(base lipgloss.Style, s string, colors []color.Color) string {
	var str string
	for i, ss := range s {
		color, _ := colorful.MakeColor(colors[i%len(colors)])
		str = str + base.Foreground(lipgloss.Color(color.Hex())).Render(string(ss))
	}
	return str
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
