package main

import (
	"encoding/json"
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
	"charm-wallet-tui/views/terra"
	"charm-wallet-tui/views/uniswap"
	"charm-wallet-tui/views/wallets"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// -------------------- VIEW --------------------

func (m model) renderConfirmDialog(prompt string, yesSelected bool) string {
	dialogBoxStyle := styles.DialogBox.Padding(1, 0)
	buttonStyle := styles.ButtonNormal.MarginTop(1)
	activeButtonStyle := styles.ButtonActive.MarginRight(2).MarginTop(1)

	question := lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(prompt)

	var okButton, cancelButton string
	if yesSelected {
		okButton = activeButtonStyle.Render("Yes")
		cancelButton = buttonStyle.Render("No")
	} else {
		okButton = buttonStyle.MarginRight(2).Render("Yes")
		cancelButton = activeButtonStyle.MarginRight(0).Render("No")
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

func (m model) renderDeleteDialog() string {
	msg := helpers.FadeString("Are you sure you want to delete the account "+helpers.ShortenAddr(m.deleteDialogAddr)+"?", "#F25D94", "#EDFF82")
	return m.renderConfirmDialog(msg, m.deleteDialogYesSelected)
}

func (m model) renderRPCDeleteDialog() string {
	msg := helpers.FadeString("Are you sure you want to delete the RPC endpoint "+m.deleteRPCDialogName+"?", "#F25D94", "#EDFF82")
	return m.renderConfirmDialog(msg, m.deleteRPCDialogYesSelected)
}

func (m *model) renderAccountListPopup() string {
	dialogBoxStyle := styles.DialogBox.Background(cPanel)

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

// txScrollbarTrack builds a vertical scrollbar track for the QR result viewport.
// Returns nil when there is nothing to scroll.
func txScrollbarTrack(vpHeight, totalLines, yOffset int) []string {
	if totalLines <= vpHeight || vpHeight <= 0 {
		return nil
	}
	thumbSize := vpHeight * vpHeight / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	maxOffset := totalLines - vpHeight
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = (yOffset * (vpHeight - thumbSize)) / maxOffset
	}
	track := make([]string, vpHeight)
	for i := range track {
		if i >= thumbTop && i < thumbTop+thumbSize {
			track[i] = "█"
		} else {
			track[i] = "░"
		}
	}
	return track
}

func (m *model) renderTxResultContent() string {
	title := styles.TitleStyle.Render("Transaction Ready To Sign (EIP-4527)")

	if m.txResultPackaging {
		return title + "\n\n" + m.spin.View() + " Packaging transaction..."
	}
	if m.txResultError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
		muteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
		return title + "\n\n" +
			errorStyle.Render("Error: "+m.txResultError) +
			"\n\n" + muteStyle.Render("Press ESC or Enter to close")
	}

	vpH := helpers.Max(3, m.h-8)
	vp := m.txQRViewport
	vp.Height = vpH
	vpContent := vp.View()

	track := txScrollbarTrack(vpH, vp.TotalLineCount(), vp.YOffset)
	if len(track) > 0 {
		trackStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
		lines := strings.Split(vpContent, "\n")
		for i := range lines {
			if i < len(track) {
				lines[i] = lines[i] + " " + trackStyle.Render(track[i])
			}
		}
		vpContent = strings.Join(lines, "\n")
	}

	result := title + "\n\n" + vpContent
	if m.txCopiedMsg != "" {
		result += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(m.txCopiedMsg)
	}
	return result
}

func (m *model) renderTxResultPanel() string {
	contentWidth := helpers.Max(0, m.w-8)
	centeredContent := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(m.renderTxResultContent())
	content := panelStyle.Width(helpers.Max(0, m.w-4)).Render(centeredContent)
	return appStyle.Render(lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		content,
	))
}

func (m *model) globalHeader() string {
	availableWidth := helpers.Max(0, m.w-8) // Account for panel padding

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

		leftSpacer := strings.Repeat(" ", helpers.Max(1, leftPadding))
		rightSpacer := strings.Repeat(" ", helpers.Max(1, rightPadding))

		headerLine = addrDisplay + leftSpacer + titleText + rightSpacer + rpcDisplay
	}

	// Add separator line
	separator := lipgloss.NewStyle().
		Foreground(cBorder).
		Render(strings.Repeat("─", availableWidth))

	return headerLine + "\n" + separator
}

func (m *model) renderPoolInfoPopup() string {
	dialogBoxStyle := styles.DialogBox.Width(72)

	// Title
	title := lipgloss.NewStyle().
		Foreground(cAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(68).
		Render("Pool Info")

	// Pool ID (full, FadeString)
	poolIDLine := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(68).
		Render(helpers.FadeString(m.poolInfoID, "#7EE787", "#82CFFD"))

	// Body
	var body string
	if m.poolInfoLoading {
		body = lipgloss.NewStyle().Foreground(cMuted).Render(m.spin.View() + " Fetching pool data…")
	} else if m.poolInfoErr != "" {
		body = lipgloss.NewStyle().Foreground(styles.CError).Render("Error: " + m.poolInfoErr)
	} else if m.poolInfoData != nil {
		raw, err := json.MarshalIndent(m.poolInfoData, "", "  ")
		if err != nil {
			body = lipgloss.NewStyle().Foreground(styles.CError).Render("Error marshalling data")
		} else {
			body = lipgloss.NewStyle().Foreground(cText).Render(string(raw))
		}
	}

	// Eth logs loading row (shown while fetching pool key)
	var keyRow string
	if m.poolInfoKeyLoading {
		keyRow = lipgloss.NewStyle().Foreground(cMuted).Render(m.spin.View() + " Loading eth logs…")
	} else if m.poolInfoKeyErr != "" {
		keyRow = lipgloss.NewStyle().Foreground(styles.CWarn).Render("⚠ " + m.poolInfoKeyErr)
	}

	// "OK" button centred in 68 cols. ButtonPrimary has Padding(0,3) → 8 cols wide.
	// Centre offset: (68-8)/2 = 30.
	okButton := styles.ButtonPrimary.MarginTop(1).Render("OK")
	btnRowCentered := lipgloss.NewStyle().Align(lipgloss.Center).Width(68).Render(okButton)

	// "Pool ID Copied" confirmation shown above OK button when copy succeeds.
	var copiedRow string
	if m.poolInfoCopied {
		copiedRow = lipgloss.NewStyle().
			Foreground(styles.CAccent).
			Align(lipgloss.Center).
			Width(68).
			Render("✓ Pool ID Copied")
	}

	rows := []string{title, poolIDLine, "", body}
	if keyRow != "" {
		rows = append(rows, keyRow)
	}
	if copiedRow != "" {
		rows = append(rows, copiedRow)
	}
	rows = append(rows, btnRowCentered)
	ui := lipgloss.JoinVertical(lipgloss.Center, rows...)
	dialog := dialogBoxStyle.Render(ui)

	// Track click areas. Dialog content starts 3 cols from left edge (border+padding).
	// Pool ID line is at row 3 from dialog top (border+padding+title).
	// OK button is centred: offset (68-8)/2 = 30 from content left.
	dialogH := lipgloss.Height(dialog)
	dialogW := lipgloss.Width(dialog)
	dialogStartX := (m.w - dialogW) / 2
	dialogStartY := (m.h - dialogH) / 2
	contentLeft := dialogStartX + 3
	m.poolInfoIDLineY = dialogStartY + 3
	m.poolInfoIDLineX1 = contentLeft
	m.poolInfoIDLineX2 = contentLeft + 68
	btnRowY := dialogStartY + dialogH - 3 // one row above bottom padding+border
	m.poolInfoOKBtnY = btnRowY
	m.poolInfoOKBtnX1 = contentLeft + 30
	m.poolInfoOKBtnX2 = contentLeft + 30 + 8

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *model) View() string {
	// Clear clickable areas for fresh render
	m.clickableAreas = nil

	// Render global header outside of page content
	globalHdr := m.globalHeader()
	headerPanel := panelStyle.Width(m.contentW).Render(globalHdr)

	// Note: Header address clickable area coordinates are set in globalHeader()

	var pageContent string
	var nav string

	switch m.activePage {
	case config.PageHome:
		// TODO: home view not implemented yet
		pageContent = panelStyle.Width(m.contentW).Render("Home view not implemented")
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
			rpcDetails := toRPCDetails(m.details)
			detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())

			// Show transaction result panel if active
			if m.activeDialog == dialogTxResult {
				detailsContent = m.renderTxResultContent()
				// Show send form if active
			} else if m.showSendForm && m.sendForm != nil {
				sendFormContent := styles.TitleStyle.Render("Send Transaction") + "\n\n" + m.sendForm.View()
				detailsContent = sendFormContent
			} else if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
				// Add send button if ETH balance > 0 and form is not active
				var sendButtonStyle lipgloss.Style
				if m.sendButtonFocused {
					sendButtonStyle = styles.ButtonActive.MarginTop(2)
				} else {
					sendButtonStyle = styles.ButtonNormal.MarginTop(2)
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
			listWidth := helpers.Max(0, (m.w*4)/10-2)
			detailsWidth := helpers.Max(0, (m.w*6)/10-2)

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
			pageContent = panelStyle.Width(m.contentW).Render(walletsContent)
		}
		nav = wallets.Nav(m.w-2, m.txIndexerActive)

		// Render delete confirmation dialog overlay
		if m.activeDialog == dialogDeleteWallet {
			// Dialog overlays the current view
			return m.renderDeleteDialog()
		}

		if m.activeDialog == dialogTxResult {
			return m.renderTxResultPanel()
		}

	case config.PageDappBrowser:
		dappBrowserContent := dapps.Render(m.w-2, m.dapps, m.selectedDappIdx)
		pageContent = panelStyle.Width(m.contentW).Render(dappBrowserContent)
		nav = dapps.Nav(m.w-2, m.txIndexerActive)

	case config.PageDetails:
		rpcDetails := toRPCDetails(m.details)
		detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())
		pageContent = panelStyle.Width(m.contentW).Render(detailsContent)
		nav = details.Nav(m.w-2, m.nicknaming, m.txIndexerActive)

	case config.PageSettings:
		settingsContent := settings.Render(m.rpcURLs, m.selectedRPCIdx)

		// Show form if in add/edit mode
		if (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
			settingsContent = styles.TitleStyle.Render("RPC Settings") + "\n\n" + m.form.View()
		}

		pageContent = panelStyle.Width(m.contentW).Render(settingsContent)
		nav = settings.Nav(m.w-2, m.settingsMode, m.txIndexerActive)

		if m.activeDialog == dialogDeleteRPC {
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
			nav = uniswap.Nav(m.w-2, m.poolEventMonitorActive, m.uniswapShowingLiquidity, m.v4BlockScanActive)
		} else if m.uniswapShowingLiquidity {
			liquidityView := uniswap.RenderLiquidity(
				m.w-2,
				m.h-8,
				m.liquidityPositions,
				m.liquidityLoading,
				m.liquidityFocusedIdx,
				m.liquidityErr,
				m.spin.View(),
			)
			pageContent = panelStyle.Width(m.contentW).Render(liquidityView)
			nav = uniswap.Nav(m.w-2, m.poolEventMonitorActive, m.uniswapShowingLiquidity, m.v4BlockScanActive)
		} else if m.poolEventMonitorActive {
			// Pool event monitor is active — show the V4 Events panel capped at 50% window height.
			// panelStyle adds 4 vertical lines (border+padding); RenderV4Events overhead is 4 more.
			// Passing (m.h/2 - 4) yields an outer panel height of exactly m.h/2.
			v4View := uniswap.RenderV4Events(m.w-2, helpers.Max(1, m.h/2-4), m.v4EventsViewport)
			v4BorderColor := styles.CBorder
			if m.focusedPanel == focusedPanelV4Events {
				v4BorderColor = styles.CAccent
			}
			pageContent = panelStyle.BorderForeground(v4BorderColor).Width(m.contentW).Render(v4View)
			nav = uniswap.Nav(m.w-2, m.poolEventMonitorActive, m.uniswapShowingLiquidity, m.v4BlockScanActive)
			// headerPanel rows + panelStyle overhead (1 border + 1 padding = 2) + title line + blank line = +4
			m.v4ViewportTop = lipgloss.Height(headerPanel) + 4
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
			pageContent = panelStyle.Width(m.contentW).Render(uniswapView)
			nav = uniswap.Nav(m.w-2, m.poolEventMonitorActive, m.uniswapShowingLiquidity, m.v4BlockScanActive)
		}

		// Show pool info popup overlay if active
		if m.activeDialog == dialogPoolInfo {
			return m.renderPoolInfoPopup()
		}

		// Show transaction result panel overlay if active
		if m.activeDialog == dialogTxResult {
			return m.renderTxResultPanel()
		}

	case config.PageTerraNullius:
		// Show claim popup overlay
		if m.activeDialog == dialogTerraClaim {
			return terra.RenderClaimPopup(m.w, m.h, m.terraNullMsgInput.View(), m.terraNullMsgError, m.terraNullFormFocused)
		}

		var terraNullDesc string
		for _, d := range m.dapps {
			if d.Name == "Terra Nullius" {
				terraNullDesc = d.Description
				break
			}
		}
		terraView := terra.Render(
			m.w-2,
			m.h-8,
			m.terraNullFocusedField,
			terraNullDesc,
			m.terraNullClaimsCount, m.terraNullClaimsLoading,
			m.terraNullClaimInput, m.terraNullClaimQuerying,
			m.terraNullLastQueriedIdx,
			m.terraNullClaimResult, m.terraNullClaimResultErr,
		)
		pageContent = panelStyle.Width(m.contentW).Render(terraView)
		nav = terra.Nav(m.w-2, m.txIndexerActive)

		if m.activeDialog == dialogTxResult {
			return m.renderTxResultPanel()
		}
	}

	// Render log panel only if enabled
	var logPanel string
	if m.logEnabled {
		// Give the log panel all remaining vertical space.
		// Total log panel height = viewportHeight + 4 (border top/bottom + title + blank line).
		usedHeight := lipgloss.Height(headerPanel) + lipgloss.Height(pageContent) + lipgloss.Height(nav)
		viewportHeight := helpers.Max(3, m.h-usedHeight-4)
		m.logViewport.Height = viewportHeight

		logFocused := m.poolEventMonitorActive && !m.uniswapShowingLiquidity && m.activePage == config.PageUniswap &&
			m.focusedPanel == focusedPanelLog
		logPanel = logview.Render(m.w, viewportHeight, m.logReady, m.logSpinner.View(), m.logViewport, logFocused)
		// Compute the on-screen Y of the log content start from the bottom of the terminal.
		// Using lipgloss.Height(logPanel) is robust when the content above overflows m.h
		// (the terminal shows the bottom m.h rows, so log border top = m.h - height(logPanel)).
		// +3 accounts for top border(1) + title(1) + blank line(1).
		m.logPanelTop = (m.h - lipgloss.Height(logPanel)) + 3
		content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav, logPanel)
		baseView := appStyle.Render(content)

		// Show account list popup overlay if active
		if m.activeDialog == dialogAccountList {
			popup := m.renderAccountListPopup()
			return popup // Popup uses lipgloss.Place internally, so just return it
		}

		return baseView
	}

	// Use lipgloss to join sections vertically (without log panel)
	content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav)
	baseView := appStyle.Render(content)

	// Show account list popup overlay if active
	if m.activeDialog == dialogAccountList {
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

func toRPCDetails(details config.WalletDetails) rpc.WalletDetails {
	rpcDetails := rpc.WalletDetails{
		Address:    details.Address,
		EthWei:     details.EthWei,
		LoadedAt:   details.LoadedAt,
		ErrMessage: details.ErrMessage,
	}
	for _, t := range details.Tokens {
		rpcDetails.Tokens = append(rpcDetails.Tokens, rpc.TokenBalance{
			Symbol:   t.Symbol,
			Decimals: t.Decimals,
			Balance:  t.Balance,
		})
	}
	return rpcDetails
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
