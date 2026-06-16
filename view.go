package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/dapps"
	"charm-wallet-tui/views/details"
	logview "charm-wallet-tui/views/log"
	"charm-wallet-tui/views/scrollbar"
	"charm-wallet-tui/views/settings"
	"charm-wallet-tui/views/terra"
	"charm-wallet-tui/views/uniswap"
	"charm-wallet-tui/views/wallets"

	"github.com/charmbracelet/lipgloss"
)

// RPCFormPopupWidth is the outer dialog box width for the RPC add/edit popup.
// RPCFormPopupInnerWidth is the huh.Form width (dialog minus border+padding).
const (
	RPCFormPopupWidth      = 64
	RPCFormPopupInnerWidth = RPCFormPopupWidth - 6
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
	dialogBoxStyle := styles.DialogBox.Background(styles.CPanel)

	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(70).
		Render("Select Account")

	accountList, popupClickableAreas, _ := wallets.RenderList(m.accounts, m.accountListSelectedIdx)

	help := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Align(lipgloss.Center).
		Width(70).
		MarginTop(1).
		Render("↑/↓: Navigate • Enter: Select • Esc: Cancel")

	ui := lipgloss.JoinVertical(lipgloss.Left, title, "", accountList, help)
	dialog := dialogBoxStyle.Render(ui)

	// Register clickable areas with popup-relative coordinates.
	const (
		dialogWidth    = 74
		renderListBaseX = 4
		renderListBaseY = 9
	)
	dialogHeight := len(m.accounts)*3 + 8
	popupStartX := (m.w - dialogWidth) / 2
	popupStartY := (m.h - dialogHeight) / 2
	contentStartX := popupStartX + 3 // border(1) + padding(2)
	contentStartY := popupStartY + 4 // border(1) + padding(1) + title(1) + blank(1)

	m.clickableAreas = nil
	for _, area := range popupClickableAreas {
		m.clickableAreas = append(m.clickableAreas, config.ClickableArea{
			X:       contentStartX + (area.X - renderListBaseX),
			Y:       contentStartY + (area.Y - renderListBaseY),
			Width:   area.Width,
			Height:  area.Height,
			Address: area.Address,
		})
	}

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog)
}


func (m *model) renderTxResultContent() string {
	titleStr := "Transaction Ready To Sign (EIP-4527)"
	if m.txApproveQRFrames != nil {
		if !m.txSwapStep {
			titleStr = "Step 1 of 2: Approve — Tab to switch steps"
		} else {
			titleStr = "Step 2 of 2: Swap — Tab to switch steps"
		}
	}
	title := styles.TitleStyle.Render(titleStr)

	if m.txResultPackaging {
		return title + "\n\n" + m.spin.View() + " Packaging transaction..."
	}
	if m.txResultError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(styles.CFail).Bold(true)
		muteStyle := lipgloss.NewStyle().Foreground(styles.CSubtle)
		return title + "\n\n" +
			errorStyle.Render("Error: "+m.txResultError) +
			"\n\n" + muteStyle.Render("Press ESC or Enter to close")
	}

	// Animated QR section: show current frame, centred, with frame counter.
	var qrSection string
	if len(m.txQRFrames) > 0 {
		qr := m.txQRFrames[m.txQRFrameIdx]
		if len(m.txQRFrames) > 1 {
			counter := lipgloss.NewStyle().
				Foreground(styles.CMuted).
				Render(fmt.Sprintf("Frame %d / %d", m.txQRFrameIdx+1, len(m.txQRFrames)))
			qrSection = qr + counter
		} else {
			qrSection = qr
		}
	}
	qrH := lipgloss.Height(qrSection)

	// Remaining vertical space goes to the scrollable text viewport.
	// Overhead: title(1) + blank(1) + qr + blank(1) + panel borders+padding(4) + slack(1) = qrH+8
	vpH := helpers.Max(3, m.h-qrH-8)
	vp := m.txQRViewport
	vp.Height = vpH
	track := scrollbar.Track(vpH, vp.TotalLineCount(), vp.YOffset)
	vpContent := scrollbar.Decorate(vp.View(), track)

	// Viewport scroll tracking: panel is vertically centred; viewport starts
	// after title(1) + blank(1) + qrSection + blank(1) + panel top border+padding(2) = qrH+5
	totalPanelH := 1 + 1 + qrH + 1 + vpH + 4 // title+blank+qr+blank+vp+borders/padding
	panelTop := (m.h - totalPanelH) / 2
	m.txQRScroll.PanelTop = panelTop + 2 + 1 + qrH + 1 // panel top + border+padding + qr + blank
	m.txQRScroll.TrackCol = m.txQRViewport.Width + 3

	result := title + "\n\n" + qrSection + "\n" + vpContent
	if m.txCopiedMsg != "" {
		result += "\n" + lipgloss.NewStyle().Foreground(styles.CSuccess).Bold(true).Render(m.txCopiedMsg)
	}
	return result
}

func (m *model) renderTxResultPanel() string {
	contentWidth := helpers.Max(0, m.w-8)
	centeredContent := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(m.renderTxResultContent())
	content := styles.PanelStyle.Width(helpers.Max(0, m.w-4)).Render(centeredContent)
	return styles.AppStyle.Render(lipgloss.Place(
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
			Foreground(styles.CAccent2).
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
			Foreground(styles.CMuted).
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
		statusColor = styles.CFail
		statusText = "No RPC"
	} else if m.rpcConnecting {
		statusIcon = "○"
		statusColor = styles.CFail
		statusText = "Connecting..."
	} else if !m.rpcConnected {
		statusIcon = "○"
		statusColor = styles.CFail
		statusText = "Connection Failed"
	} else {
		statusIcon = "●"
		statusColor = styles.CAccent
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
		Foreground(styles.CAccent).
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
		Foreground(styles.CBorder).
		Render(strings.Repeat("─", availableWidth))

	return headerLine + "\n" + separator
}

func (m *model) renderPoolInfoPopup() string {
	dialogBoxStyle := styles.DialogBox.Width(72)

	// Title
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
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
		body = lipgloss.NewStyle().Foreground(styles.CMuted).Render(m.spin.View() + " Fetching pool data…")
	} else if m.poolInfoErr != "" {
		body = lipgloss.NewStyle().Foreground(styles.CError).Render("Error: " + m.poolInfoErr)
	} else if m.poolInfoData != nil {
		raw, err := json.MarshalIndent(m.poolInfoData, "", "  ")
		if err != nil {
			body = lipgloss.NewStyle().Foreground(styles.CError).Render("Error marshalling data")
		} else {
			body = lipgloss.NewStyle().Foreground(styles.CText).Render(string(raw))
		}
	}

	// Eth logs loading row (shown while fetching pool key)
	var keyRow string
	if m.poolInfoKeyLoading {
		keyRow = lipgloss.NewStyle().Foreground(styles.CMuted).Render(m.spin.View() + " Loading eth logs…")
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

// renderActiveOverlay returns a full-screen overlay string when a dialog is open,
// or "" when no overlay applies. Checked before any page content is rendered.
func (m *model) renderActiveOverlay() string {
	if m.activePage == config.PageSettings && (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		return m.renderRPCFormPopup()
	}

	switch m.activeDialog {
	case dialogTxResult:
		return m.renderTxResultPanel()
	case dialogScanTx:
		return m.renderScanTxPanel()
	case dialogPasteSignedTx:
		return m.renderPasteSignedTxPanel()
	case dialogDeleteWallet:
		return m.renderDeleteDialog()
	case dialogDeleteRPC:
		return m.renderRPCDeleteDialog()
	case dialogAccountList:
		return m.renderAccountListPopup()
	case dialogPoolInfo:
		return m.renderPoolInfoPopup()
	case dialogTerraClaim:
		return terra.RenderClaimPopup(m.w, m.h, m.terraNullMsgInput.View(), m.terraNullMsgError, m.terraNullFormFocused)
	case dialogEditWallet:
		return m.renderEditWalletDialog()
	case dialogAddWallet:
		return m.renderAddWalletDialog()
	}
	return ""
}

func (m *model) renderAddWalletDialog() string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(54).
		Render("Add Account")

	inputView := m.input.View() + "\n\n" + m.nicknameInput.View()

	if m.ensLookupActive {
		inputView += "\n" + m.spin.View() + " ENS lookup…"
	}

	hints := lipgloss.NewStyle().Foreground(styles.CMuted).Render(
		styles.HotkeyStyle.Render("Tab") + " next   " +
			styles.HotkeyStyle.Render("Enter") + " save   " +
			styles.HotkeyStyle.Render("Esc") + " cancel   " +
			styles.HotkeyStyle.Render("Ctrl+v") + " paste",
	)

	var errLine string
	if m.addError != "" && time.Since(m.addErrTime) < 3*time.Second {
		errLine = "\n" + lipgloss.NewStyle().Foreground(styles.CWarn).Bold(true).Render(m.addError)
	}

	ui := lipgloss.JoinVertical(lipgloss.Left, title, "", inputView, "", hints+errLine)

	dialog := styles.DialogBox.Padding(1, 2).Render(ui)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *model) renderEditWalletDialog() string {
	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(54).
		Render("Edit Account")

	inputView := m.input.View() + "\n\n" + m.nicknameInput.View()

	if m.ensLookupActive {
		inputView += "\n" + m.spin.View() + " ENS lookup…"
	}

	hints := lipgloss.NewStyle().Foreground(styles.CMuted).Render(
		styles.HotkeyStyle.Render("Tab") + " next   " +
			styles.HotkeyStyle.Render("Enter") + " save   " +
			styles.HotkeyStyle.Render("Esc") + " cancel   " +
			styles.HotkeyStyle.Render("Ctrl+v") + " paste",
	)

	var errLine string
	if m.addError != "" && time.Since(m.addErrTime) < 3*time.Second {
		errLine = "\n" + lipgloss.NewStyle().Foreground(styles.CWarn).Bold(true).Render(m.addError)
	}

	ui := lipgloss.JoinVertical(lipgloss.Left, title, "", inputView, "", hints+errLine)

	dialog := styles.DialogBox.Padding(1, 2).Render(ui)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *model) renderRPCFormPopup() string {
	var titleText string
	if m.settingsMode == "edit" {
		titleText = "Edit RPC Endpoint"
	} else {
		titleText = "Add RPC Endpoint"
	}

	title := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(RPCFormPopupWidth - 8).
		Render(titleText)

	hints := lipgloss.NewStyle().Foreground(styles.CMuted).Render(
		styles.HotkeyStyle.Render("Tab") + " next   " +
			styles.HotkeyStyle.Render("Enter") + " save   " +
			styles.HotkeyStyle.Render("Esc") + " cancel",
	)

	ui := lipgloss.JoinVertical(lipgloss.Left, title, "", m.form.View(), "", hints)
	dialog := styles.DialogBox.Padding(1, 2).Render(ui)

	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *model) View() string {
	m.clickableAreas = nil
	globalHdr := m.globalHeader()
	headerPanel := styles.PanelStyle.Width(m.contentW).Render(globalHdr)

	if overlay := m.renderActiveOverlay(); overlay != "" {
		return overlay
	}

	pageContent, nav := m.renderPage(headerPanel)

	var logPanel string
	if m.logEnabled {
		usedHeight := lipgloss.Height(headerPanel) + lipgloss.Height(pageContent) + lipgloss.Height(nav)
		viewportHeight := helpers.Max(3, m.h-usedHeight-4)
		m.logViewport.Height = viewportHeight
		logFocused := m.poolEventMonitorActive && !m.uniswapShowingLiquidity &&
			m.activePage == config.PageUniswap && m.focusedPanel == focusedPanelLog
		logPanel = logview.Render(m.w, viewportHeight, m.logReady, m.logSpinner.View(), m.logViewport, logFocused)
		// log border top = m.h - height(logPanel); +3 for top border(1)+title(1)+blank(1)
		m.logScroll.PanelTop = (m.h - lipgloss.Height(logPanel)) + 3
		m.logScroll.TrackCol = m.logViewport.Width + 3
		content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav, logPanel)
		return styles.AppStyle.Render(content)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav)
	return styles.AppStyle.Render(content)
}

func (m *model) renderPage(headerPanel string) (pageContent, nav string) {
	switch m.activePage {
	case config.PageHome:
		return styles.PanelStyle.Width(m.contentW).Render("Home view not implemented"), ""

	case config.PageWallets:
		return m.renderWalletsPage(headerPanel)

	case config.PageDappBrowser:
		c := dapps.Render(m.w-2, m.dapps, m.selectedDappIdx)
		return styles.PanelStyle.Width(m.contentW).Render(c), dapps.Nav(m.w-2, m.txIndexerActive)

	case config.PageDetails:
		c := details.Render(m.details, m.accounts, m.loading, m.copiedMsg, m.spin.View(), m.chainID())
		return styles.PanelStyle.Width(m.contentW).Render(c), details.Nav(m.w-2, m.nicknaming, m.txIndexerActive)

	case config.PageSettings:
		c := settings.Render(m.rpcURLs, m.selectedRPCIdx)
		return styles.PanelStyle.Width(m.contentW).Render(c), settings.Nav(m.w-2, m.settingsMode, m.txIndexerActive)

	case config.PageUniswap:
		return m.renderUniswapPage(headerPanel)

	case config.PageTerraNullius:
		return m.renderTerraPage()
	}
	return "", ""
}

func (m *model) renderWalletsPage(headerPanel string) (pageContent, nav string) {
	walletsContent, walletsClickableAreas := wallets.Render(m.accounts, m.selectedWallet, m.addError)
	for _, area := range walletsClickableAreas {
		m.clickableAreas = append(m.clickableAreas, config.ClickableArea{
			X: area.X, Y: area.Y + 1, Width: area.Width, Height: area.Height, Address: area.Address,
		})
	}

	nav = wallets.Nav(m.w-2, m.txIndexerActive)

	if !m.detailsInWallets || len(m.accounts) == 0 {
		return styles.PanelStyle.Width(m.contentW).Render(walletsContent), nav
	}

	detailsContent := details.Render(m.details, m.accounts, m.loading, m.copiedMsg, m.spin.View(), m.chainID())

	var detailsBaseH int
	if m.showSendForm && m.sendForm != nil {
		detailsContent = styles.TitleStyle.Render("Send Transaction") + "\n\n" + m.sendForm.View()
		m.sendBtnW = 0
	} else if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
		detailsBaseH = lipgloss.Height(detailsContent)
		var btnStyle lipgloss.Style
		if m.sendButtonFocused || m.sendButtonHovered {
			btnStyle = styles.ButtonActive.MarginTop(2)
		} else {
			btnStyle = styles.ButtonNormal.MarginTop(2)
		}
		detailsContent += "\n\n" + btnStyle.Render("Send")
		hint := "Press Tab to select"
		if m.sendButtonFocused {
			hint = "Press Enter to send"
		}
		detailsContent += "\n" + lipgloss.NewStyle().Foreground(styles.CSubtle).MarginTop(1).Render(hint)
	} else {
		m.sendBtnW = 0
	}

	listWidth := helpers.Max(0, (m.w*4)/10-2)
	detailsWidth := helpers.Max(0, (m.w*6)/10-2)
	leftPanel := styles.PanelStyle.Width(listWidth).Render(walletsContent)
	leftPanelHeight := lipgloss.Height(leftPanel)

	if detailsBaseH > 0 {
		m.sendBtnX = listWidth + 5
		m.sendBtnY = lipgloss.Height(headerPanel) + 2 + detailsBaseH + 3
		m.sendBtnW = 10
	}

	rightPanel := styles.PanelStyle.Width(detailsWidth+1).Height(leftPanelHeight-2).Render(detailsContent)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel), nav
}

func (m *model) renderUniswapPage(headerPanel string) (pageContent, nav string) {
	tokens := m.buildTokenList()
	navStr := uniswap.Nav(m.w-2, m.poolEventMonitorActive, m.uniswapShowingLiquidity, m.v4BlockScanActive)

	if m.uniswapShowingSelector {
		c := uniswap.RenderTokenSelector(m.w, m.h-8, tokens, m.uniswapSelectorIdx, m.uniswapSelectorFor == 0)
		return c, navStr
	}
	if m.uniswapShowingLiquidity {
		c := uniswap.RenderLiquidity(m.w-2, m.h-8, m.liquidityPositions, m.liquidityLoading,
			m.liquidityFocusedIdx, m.liquidityErr, m.spin.View())
		return styles.PanelStyle.Width(m.contentW).Render(c), navStr
	}
	if m.poolEventMonitorActive {
		// PanelStyle adds 4 vertical lines; RenderV4Events overhead is 4 — so m.h/2-4 yields half-height.
		v4View := uniswap.RenderV4Events(m.w-2, helpers.Max(1, m.h/2-4), m.v4EventsViewport)
		borderColor := styles.CBorder
		if m.focusedPanel == focusedPanelV4Events {
			borderColor = styles.CAccent
		}
		c := styles.PanelStyle.BorderForeground(borderColor).Width(m.contentW).Render(v4View)
		m.v4Scroll.PanelTop = lipgloss.Height(headerPanel) + 4
		m.v4Scroll.TrackCol = m.v4EventsViewport.Width + 3
		return c, navStr
	}
	c := uniswap.Render(m.w-2, m.h-8, tokens,
		m.uniswapFromTokenIdx, m.uniswapToTokenIdx,
		m.uniswapFromAmount, m.uniswapToAmount,
		m.uniswapFocusedField, m.uniswapEstimating, m.uniswapPriceImpactWarn)
	return styles.PanelStyle.Width(m.contentW).Render(c), navStr
}

func (m *model) renderTerraPage() (pageContent, nav string) {
	var terraNullDesc string
	for _, d := range m.dapps {
		if d.Name == "Terra Nullius" {
			terraNullDesc = d.Description
			break
		}
	}
	c := terra.Render(m.w-2, m.h-8, m.terraNullFocusedField, terraNullDesc,
		m.terraNullClaimsCount, m.terraNullClaimsLoading,
		m.terraNullClaimInput, m.terraNullClaimQuerying,
		m.terraNullLastQueriedIdx, m.terraNullClaimResult, m.terraNullClaimResultErr)
	return styles.PanelStyle.Width(m.contentW).Render(c), terra.Nav(m.w-2, m.txIndexerActive)
}


