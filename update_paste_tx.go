package main

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/atotto/clipboard"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// pasteSignedTxDialogWidth is the fixed width of the paste-tx popup across all phases.
const pasteSignedTxDialogWidth = 78

// tempPasteSignedTxHex binds the huh.Form text field. Package-level, per the
// project's "Huh Forms" convention (see tempRPCFormName in update_settings.go).
var tempPasteSignedTxHex string

// createPasteSignedTxForm builds the paste form: a single multi-line text
// input styled like the Settings forms (Catppuccin theme). The live preview
// is rendered separately in renderPasteTxFormPhase rather than via a huh.Note
// so that (a) the Note's required keypress is avoided, and (b) the Init cmd
// can be returned and actually executed by the Tea runtime.
func (m *model) createPasteSignedTxForm(initial string) tea.Cmd {
	tempPasteSignedTxHex = initial

	field := huh.NewText().
		Title("Paste the signed transaction in the input below").
		Value(&tempPasteSignedTxHex).
		Lines(3).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("paste a signed transaction")
			}
			_, err := rpc.DecodeSignedRawTx(s)
			return err
		})

	m.pasteTxFormField = field
	m.pasteTxButtonFocused = false
	m.pasteTxForm = huh.NewForm(
		huh.NewGroup(field),
	).WithWidth(pasteSignedTxDialogWidth - 6).WithTheme(huh.ThemeCatppuccin())

	return m.pasteTxForm.Init()
}

// formatSignedTxPreview renders the live "JSON and human readable tx" preview
// shown above the paste input as the user types or pastes.
func formatSignedTxPreview(rawHex string) string {
	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

	trimmed := strings.TrimSpace(rawHex)
	if trimmed == "" {
		return muteStyle.Render("Paste a signed transaction to see a preview…")
	}

	decoded, err := rpc.DecodeSignedRawTx(trimmed)
	if err != nil {
		return lipgloss.NewStyle().Foreground(styles.CError).Render("Could not parse transaction: " + err.Error())
	}

	labelStyle := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(styles.CText)
	row := func(label, value string) string {
		return labelStyle.Render(label+": ") + valueStyle.Render(value)
	}

	to := decoded.To
	if to == "" {
		to = "(contract creation)"
	}

	gasRow := row("Gas", fmt.Sprintf("%d @ %s", decoded.Gas, decoded.GasPriceHuman))
	if decoded.IsEIP1559 {
		gasRow = row("Gas", fmt.Sprintf("%d limit, max %s / priority %s", decoded.Gas, decoded.MaxFeeHuman, decoded.PriorityFeeHuman))
	}

	summary := strings.Join([]string{
		row("Hash", decoded.Hash),
		row("From", decoded.From),
		row("To", to),
		row("Value", decoded.ValueHuman),
		row("Nonce", fmt.Sprintf("%d", decoded.Nonce)),
		gasRow,
		row("Chain ID", decoded.ChainID.String()),
	}, "\n")

	return summary + "\n\n" + labelStyle.Render("JSON:") + "\n" + muteStyle.Render(decoded.JSON)
}

// activeRPCLabel returns the friendly name of the active RPC endpoint,
// falling back to its URL when no name is set.
func (m *model) activeRPCLabel() string {
	for _, u := range m.rpcURLs {
		if u.Active {
			if u.Name != "" {
				return u.Name
			}
			return u.URL
		}
	}
	return m.rpcURL
}

// etherscanTxURL returns the correct block explorer transaction URL for the
// given chain ID, falling back to mainnet Etherscan for unknown/nil chain IDs.
func etherscanTxURL(chainID *big.Int, txHash string) string {
	return helpers.ExplorerBaseURL(chainID) + "/tx/" + txHash
}

// openPasteSignedTxDialog releases the webcam (if active) and opens the
// paste-signed-transaction popup, focused and ready for input. This is the
// "default activated and on focus if no camera is detected or any camera
// error occurs" entry point, and also the target of the visible
// "Paste a signed transaction" button when the camera is working.
func (m *model) openPasteSignedTxDialog() (tea.Model, tea.Cmd) {
	return m.openPasteSignedTxDialogWithHex("")
}

// openPasteSignedTxDialogWithHex is openPasteSignedTxDialog with the paste
// field prefilled — used when a webcam-scanned QR code already decodes as a
// complete signed transaction, so the user lands straight on the
// human-readable preview/Submit step instead of an empty paste box.
func (m *model) openPasteSignedTxDialogWithHex(initial string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if m.activeDialog == dialogScanTx {
		_, closeCmd := m.closeScanTxDialog()
		if closeCmd != nil {
			cmds = append(cmds, closeCmd)
		}
	}

	m.activeDialog = dialogPasteSignedTx
	m.pasteTxPhase = pasteTxPhaseForm
	m.pasteTxHash = ""
	m.pasteTxSendErr = ""
	m.pasteTxCountdown = 0
	m.pasteTxPollErr = ""
	m.pasteTxOnChainInfo = nil
	m.pasteTxChainID = nil
	m.pasteTxHashLineY, m.pasteTxHashLineX1, m.pasteTxHashLineX2 = 0, 0, 0
	cmds = append(cmds, m.createPasteSignedTxForm(initial))

	return m, tea.Batch(cmds...)
}

// closePasteSignedTxDialog resets all paste-tx state and returns to whatever
// page the transaction originated from — m.activePage never changes while
// this overlay is open, so dropping back to dialogNone is enough.
func (m *model) closePasteSignedTxDialog() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	m.pasteTxForm = nil
	m.pasteTxButtonFocused = false
	m.pasteTxPhase = pasteTxPhaseForm
	m.pasteTxHash = ""
	m.pasteTxSendErr = ""
	m.pasteTxCountdown = 0
	m.pasteTxPollErr = ""
	m.pasteTxOnChainInfo = nil
	tempPasteSignedTxHex = ""
	return m, nil
}

// handlePasteSignedTxMsg is the single entry point for dialogPasteSignedTx,
// dispatched from Update when that dialog is active.
func (m *model) handlePasteSignedTxMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case signedTxBroadcastMsg:
		if m.pasteTxPhase != pasteTxPhaseSending {
			return m, nil
		}
		if msg.err != nil {
			m.pasteTxSendErr = msg.err.Error()
			m.logError("Broadcast failed: " + msg.err.Error())
			return m, nil
		}
		m.pasteTxHash = msg.txHash
		m.pasteTxPhase = pasteTxPhasePolling
		m.pasteTxCountdown = 30
		m.logSuccess("Broadcast signed transaction — hash " + msg.txHash)
		return m, tea.Batch(
			pollTxOnChain(m.ethClient, msg.txHash),
			pasteTxCountdownTick(),
		)

	case signedTxPollResultMsg:
		if m.pasteTxPhase != pasteTxPhasePolling {
			return m, nil
		}
		if msg.err != nil {
			m.pasteTxPollErr = msg.err.Error()
			m.logWarn("On-chain check failed: " + msg.err.Error())
			return m, nil
		}
		m.pasteTxPollErr = ""
		if msg.found && msg.info != nil {
			m.pasteTxOnChainInfo = msg.info
			m.pasteTxPhase = pasteTxPhaseResult
			m.logSuccess(fmt.Sprintf("Transaction confirmed in block %d (%s)", msg.info.BlockNumber, msg.info.Status))
		}
		return m, nil

	case signedTxCountdownTickMsg:
		if m.pasteTxPhase != pasteTxPhasePolling {
			return m, nil
		}
		m.pasteTxCountdown--
		if m.pasteTxCountdown <= 0 {
			m.pasteTxCountdown = 30
			return m, tea.Batch(pollTxOnChain(m.ethClient, m.pasteTxHash), pasteTxCountdownTick())
		}
		return m, pasteTxCountdownTick()

	case tea.KeyMsg:
		return m.handlePasteSignedTxKey(msg)
	}

	// Forward anything else (huh's internal field/cursor/spinner messages)
	// to the form while the paste form is the active phase.
	if m.pasteTxPhase == pasteTxPhaseForm && m.pasteTxForm != nil {
		form, cmd := m.pasteTxForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.pasteTxForm = f
		}
		return m, cmd
	}
	return m, nil
}

// submitPasteTxIfValid decodes and broadcasts the currently pasted hex if it
// validates, returning ok=false (no state change) if it doesn't. Shared by
// the textarea's single-Enter shortcut, huh's own StateCompleted submission,
// and the popup's mouse-clickable Submit button.
func (m *model) submitPasteTxIfValid() (tea.Model, tea.Cmd, bool) {
	raw := strings.TrimSpace(tempPasteSignedTxHex)
	decoded, err := rpc.DecodeSignedRawTx(raw)
	if err != nil {
		return m, nil, false
	}
	m.pasteTxChainID = decoded.ChainID
	m.pasteTxForm = nil
	m.pasteTxPhase = pasteTxPhaseSending
	m.logInfo("Broadcasting pasted signed transaction…")
	return m, broadcastSignedTx(m.ethClient, raw), true
}

// handlePasteSignedTxKey handles key presses for dialogPasteSignedTx,
// branching by phase: form keys drive huh, the later phases mostly wait for
// "any key" / ESC to dismiss.
func (m *model) handlePasteSignedTxKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.pasteTxPhase {
	case pasteTxPhaseForm:
		if msg.String() == "esc" {
			return m.closePasteSignedTxDialog()
		}

		if m.pasteTxButtonFocused {
			// Focus is on the Submit button, not the textarea — handle the
			// button's own keys here rather than forwarding to huh.
			switch msg.String() {
			case "enter", " ":
				updated, cmd, _ := m.submitPasteTxIfValid()
				return updated, cmd
			case "tab", "shift+tab":
				m.pasteTxButtonFocused = false
				return m, m.pasteTxFormField.Focus()
			}
			return m, nil
		}

		if msg.String() == "ctrl+v" {
			// Read the clipboard ourselves, synchronously — bypasses the
			// textarea's async Paste cmd (clipboard.ReadAll in a goroutine,
			// delivered later via an unexported pasteMsg), which otherwise
			// races a fast Enter against an as-yet-unpopulated field.
			if text, err := clipboard.ReadAll(); err == nil && text != "" {
				return m, m.createPasteSignedTxForm(text)
			}
			return m, nil
		}
		// huh.Text is a multi-line textarea: Enter inserts a newline rather
		// than completing the form. Intercept Enter here when the content
		// already validates so the user only needs a single Enter to submit.
		if msg.String() == "enter" {
			if updated, cmd, ok := m.submitPasteTxIfValid(); ok {
				return updated, cmd
			}
			// Content not yet valid — fall through so huh can surface the
			// validation error in the textarea UI.
		}
		// Tab would otherwise complete the form directly (huh treats Tab on
		// the last/only field as submit); intercept it so Tab stops on the
		// Submit button first instead.
		if msg.String() == "tab" {
			m.pasteTxButtonFocused = true
			return m, m.pasteTxFormField.Blur()
		}
		if m.pasteTxForm == nil {
			return m, nil
		}
		form, cmd := m.pasteTxForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.pasteTxForm = f
			switch m.pasteTxForm.State {
			case huh.StateCompleted:
				if updated, cmd, ok := m.submitPasteTxIfValid(); ok {
					return updated, cmd
				}
				return m, nil
			case huh.StateAborted:
				return m.closePasteSignedTxDialog()
			}
		}
		return m, cmd

	case pasteTxPhaseSending:
		if m.pasteTxSendErr != "" {
			return m.closePasteSignedTxDialog() // any key dismisses the broadcast error
		}
		if msg.String() == "esc" {
			return m.closePasteSignedTxDialog()
		}
		return m, nil

	case pasteTxPhasePolling:
		if msg.String() == "esc" {
			return m.closePasteSignedTxDialog()
		}
		return m, nil

	case pasteTxPhaseResult:
		_, closeCmd := m.closePasteSignedTxDialog() // "press any key to return"
		return m, tea.Batch(closeCmd, m.loadSelectedWalletDetailsFresh())
	}
	return m, nil
}

// -------------------- RENDERING --------------------

// renderPasteSignedTxPanel dispatches to the renderer for the current phase
// of the paste-signed-transaction popup.
func (m *model) renderPasteSignedTxPanel() string {
	switch m.pasteTxPhase {
	case pasteTxPhaseForm:
		return m.renderPasteTxFormPhase()
	case pasteTxPhaseSending:
		return m.renderPasteTxSendingPhase()
	case pasteTxPhasePolling:
		return m.renderPasteTxPollingPhase()
	case pasteTxPhaseResult:
		return m.renderPasteTxResultPhase()
	}
	return ""
}

func (m *model) pasteTxDialogTitle(text string) string {
	return lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(pasteSignedTxDialogWidth - 4).
		Render(text)
}

func (m *model) renderPasteTxFormPhase() string {
	if m.pasteTxForm == nil {
		return ""
	}
	formView := m.pasteTxForm.View()

	btnStyle := styles.ButtonNormal
	if m.hoveredRegionID == "pasteTx.submit" || m.pasteTxButtonFocused {
		btnStyle = styles.ButtonActive
	}
	submitBtn := btnStyle.Render("Submit")
	btnRow := lipgloss.NewStyle().
		Width(pasteSignedTxDialogWidth - 4).
		Align(lipgloss.Center).
		Render(submitBtn)

	title := m.pasteTxDialogTitle("◉ Paste Signed Transaction")
	// Constrained to the dialog's content width: formatSignedTxPreview's JSON
	// dump line is otherwise unwrapped and can render far wider than the
	// dialog once a valid tx is pasted, which throws off every width-based
	// offset computed below (button centering, hit-test geometry).
	preview := lipgloss.NewStyle().Width(pasteSignedTxDialogWidth - 4).Render(formatSignedTxPreview(tempPasteSignedTxHex))
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		preview,
		"",
		formView,
		"",
		btnRow,
	)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(body)

	dialogW := lipgloss.Width(dialog)
	dialogH := lipgloss.Height(dialog)
	startX := (m.w - dialogW) / 2
	startY := (m.h - dialogH) / 2
	contentLeft := startX + 3 // border(1) + padding-left(2), DialogBox default Padding(1,2)

	btnRowOffset := lipgloss.Height(title) + 1 + lipgloss.Height(preview) + 1 + lipgloss.Height(formView) + 1
	btnRowY := startY + 2 + btnRowOffset
	rowWidth := pasteSignedTxDialogWidth - 4
	btnX1 := contentLeft + (rowWidth-lipgloss.Width(submitBtn))/2
	btnX2 := btnX1 + lipgloss.Width(submitBtn)
	m.registerRegion("pasteTx.submit", uiRegionButton, btnX1, btnRowY, btnX2, btnRowY+1, func(m *model) (tea.Model, tea.Cmd) {
		updated, cmd, _ := m.submitPasteTxIfValid()
		return updated, cmd
	})

	return styles.AppStyle.Render(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog))
}

func (m *model) renderPasteTxSendingPhase() string {
	var body string
	if m.pasteTxSendErr != "" {
		body = lipgloss.NewStyle().Foreground(styles.CError).Render("Broadcast failed: "+m.pasteTxSendErr) +
			"\n\n" + lipgloss.NewStyle().Foreground(styles.CSubtle).Render("Press any key to return")
	} else {
		body = m.spin.View() + lipgloss.NewStyle().Foreground(styles.CMuted).Render(" Sending transaction to ") +
			lipgloss.NewStyle().Foreground(styles.CAccent2).Render(m.activeRPCLabel()) +
			lipgloss.NewStyle().Foreground(styles.CMuted).Render(" …")
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		m.pasteTxDialogTitle("Message Sent"),
		"",
		body,
	)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(content)
	return styles.AppStyle.Render(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog))
}

func (m *model) renderPasteTxPollingPhase() string {
	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

	hashLabel := muteStyle.Render("Tx hash: ")
	hashStyle := lipgloss.NewStyle().Foreground(styles.CAccent).Underline(true)
	hashLine := hashLabel + hashStyle.Render(m.pasteTxHash)
	status := m.spin.View() + muteStyle.Render(" Watching the chain for this transaction…")
	countdown := muteStyle.Render(fmt.Sprintf("Next check in %ds", m.pasteTxCountdown))

	const hashLineIdx = 2 // index of hashLine within rows below
	rows := []string{
		m.pasteTxDialogTitle("Message Sent — Awaiting Confirmation"),
		"",
		hashLine,
		"",
		status,
		countdown,
	}
	if m.pasteTxPollErr != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(styles.CWarn).Render("⚠ "+m.pasteTxPollErr))
	}
	rows = append(rows, "", lipgloss.NewStyle().Foreground(styles.CSubtle).Render("Double-click or ctrl-click hash → Etherscan   ESC to stop watching and return"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(content)

	// Track the hash's click area. DialogBox uses Border(1) + Padding(1, 2),
	// so content starts 2 rows / 3 cols from the dialog's screen origin
	// (mirrors renderPoolInfoPopup's dialogStartX/Y derivation in view.go).
	dialogH := lipgloss.Height(dialog)
	dialogW := lipgloss.Width(dialog)
	dialogStartX := (m.w - dialogW) / 2
	dialogStartY := (m.h - dialogH) / 2
	contentLeft := dialogStartX + 3
	m.pasteTxHashLineY = dialogStartY + 2 + hashLineIdx
	m.pasteTxHashLineX1 = contentLeft + lipgloss.Width(hashLabel)
	m.pasteTxHashLineX2 = m.pasteTxHashLineX1 + lipgloss.Width(m.pasteTxHash)

	return styles.AppStyle.Render(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog))
}

func (m *model) renderPasteTxResultPhase() string {
	info := m.pasteTxOnChainInfo
	if info == nil {
		return ""
	}

	statusColor := styles.CAccent
	if info.Status != "Success" {
		statusColor = styles.CError
	}

	labelStyle := lipgloss.NewStyle().Foreground(styles.CMuted)
	valueStyle := lipgloss.NewStyle().Foreground(styles.CText)
	row := func(label, value string, valStyle lipgloss.Style) string {
		return labelStyle.Render(fmt.Sprintf("%-20s", label+":")) + valStyle.Render(value)
	}

	to := info.To
	if to == "" {
		to = "(contract creation)"
	}

	rows := []string{
		m.pasteTxDialogTitle("Transaction Confirmed"),
		"",
		row("Status", info.Status, lipgloss.NewStyle().Foreground(statusColor).Bold(true)),
	}
	if info.Status == "Failed" && info.RevertReason != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(styles.CError).Width(pasteSignedTxDialogWidth-4).Render("Reason: "+info.RevertReason))
	}
	rows = append(rows,
		row("Hash", info.Hash, valueStyle),
		row("Block", fmt.Sprintf("%d  (%s)", info.BlockNumber, info.BlockHash), valueStyle),
		row("Confirmations", fmt.Sprintf("%d", info.Confirmations), valueStyle),
		row("From", info.From, valueStyle),
		row("To", to, valueStyle),
		row("Value", info.ValueHuman, valueStyle),
		row("Nonce", fmt.Sprintf("%d", info.Nonce), valueStyle),
		row("Gas Used", fmt.Sprintf("%d", info.GasUsed), valueStyle),
		row("Effective Gas Price", info.EffectiveGasPrice, valueStyle),
		row("Tx Index", fmt.Sprintf("%d", info.TransactionIndex), valueStyle),
		"",
		lipgloss.NewStyle().Foreground(styles.CSubtle).Align(lipgloss.Center).Width(pasteSignedTxDialogWidth-4).Render("Press any key to return"),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(content)
	return styles.AppStyle.Render(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog))
}
