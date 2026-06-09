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

// createPasteSignedTxForm builds the paste form: a live preview note above a
// multi-line text input, styled like the Settings forms (Catppuccin theme).
func (m *model) createPasteSignedTxForm(initial string) {
	tempPasteSignedTxHex = initial

	m.pasteTxForm = huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Transaction Preview").
				DescriptionFunc(func() string {
					return formatSignedTxPreview(tempPasteSignedTxHex)
				}, &tempPasteSignedTxHex),

			huh.NewText().
				Title("Paste the signed transaction in the input below").
				Value(&tempPasteSignedTxHex).
				Lines(3).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("paste a signed transaction")
					}
					_, err := rpc.DecodeSignedRawTx(s)
					return err
				}),
		),
	).WithWidth(pasteSignedTxDialogWidth - 6).WithTheme(huh.ThemeCatppuccin())

	m.pasteTxForm.Init()
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

	summary := strings.Join([]string{
		row("Hash", decoded.Hash),
		row("From", decoded.From),
		row("To", to),
		row("Value", decoded.ValueHuman),
		row("Nonce", fmt.Sprintf("%d", decoded.Nonce)),
		row("Gas", fmt.Sprintf("%d @ %s", decoded.Gas, decoded.GasPriceHuman)),
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
	m.createPasteSignedTxForm("")

	return m, tea.Batch(cmds...)
}

// closePasteSignedTxDialog resets all paste-tx state and returns to whatever
// page the transaction originated from — m.activePage never changes while
// this overlay is open, so dropping back to dialogNone is enough.
func (m *model) closePasteSignedTxDialog() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	m.pasteTxForm = nil
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

// handlePasteSignedTxKey handles key presses for dialogPasteSignedTx,
// branching by phase: form keys drive huh, the later phases mostly wait for
// "any key" / ESC to dismiss.
func (m *model) handlePasteSignedTxKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.pasteTxPhase {
	case pasteTxPhaseForm:
		if msg.String() == "esc" {
			return m.closePasteSignedTxDialog()
		}
		if msg.String() == "ctrl+v" {
			// Read the clipboard ourselves, synchronously — bypasses the
			// textarea's async Paste cmd (clipboard.ReadAll in a goroutine,
			// delivered later via an unexported pasteMsg), which otherwise
			// races a fast Enter against an as-yet-unpopulated field.
			if text, err := clipboard.ReadAll(); err == nil && text != "" {
				m.createPasteSignedTxForm(text)
			}
			return m, nil
		}
		if m.pasteTxForm == nil {
			return m, nil
		}
		form, cmd := m.pasteTxForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.pasteTxForm = f
			switch m.pasteTxForm.State {
			case huh.StateCompleted:
				raw := tempPasteSignedTxHex
				if decoded, err := rpc.DecodeSignedRawTx(raw); err == nil {
					m.pasteTxChainID = decoded.ChainID
				}
				m.pasteTxForm = nil
				m.pasteTxPhase = pasteTxPhaseSending
				m.logInfo("Broadcasting pasted signed transaction…")
				return m, broadcastSignedTx(m.ethClient, raw)
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
		return m.closePasteSignedTxDialog() // "press any key to return"
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
	body := lipgloss.JoinVertical(lipgloss.Left,
		m.pasteTxDialogTitle("◉ Paste Signed Transaction"),
		"",
		m.pasteTxForm.View(),
	)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(body)
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
		lipgloss.NewStyle().Foreground(styles.CSubtle).Align(lipgloss.Center).Width(pasteSignedTxDialogWidth - 4).Render("Press any key to return"),
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	dialog := styles.DialogBox.Width(pasteSignedTxDialogWidth).Render(content)
	return styles.AppStyle.Render(lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, dialog))
}
