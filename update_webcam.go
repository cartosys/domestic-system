package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/webcam/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// openScanTxDialog starts the webcam and transitions to dialogScanTx.
// Call this when the user presses Enter on dialogTxResult.
func (m *model) openScanTxDialog() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogScanTx
	m.webcamActive = true
	m.webcamRendered = ""
	m.webcamScanLog = nil
	m.webcamErrStr = ""
	return m, tea.Cmd(openWebcamCmd)
}

// closeScanTxDialog stops the webcam and returns to dialogNone.
func (m *model) closeScanTxDialog() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	m.webcamActive = false
	if m.webcamCam != nil {
		cam := m.webcamCam
		m.webcamCam = nil
		m.webcamFrameCh = nil
		return m, func() tea.Msg {
			cam.Close()
			return nil
		}
	}
	m.webcamFrameCh = nil
	return m, nil
}

func (m *model) handleScanTxKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m.closeScanTxDialog()
	}
	return m, nil
}

// handleWebcamMsg processes webcam-related messages.
func (m *model) handleWebcamMsg(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {

	case webcamReadyMsg:
		m.webcamCam = msg.cam
		m.webcamFrameCh = msg.ch
		return m, waitForWebcamFrame(msg.ch), true

	case webcamFrameMsg:
		if !m.webcamActive {
			return m, nil, true
		}
		// Render the frame as half-blocks; inner width = panel width - 2 (border)
		videoW := m.w - 6
		videoH := (m.h / 2) - 2
		if videoW > 0 && videoH > 0 {
			m.webcamRendered = render.ImageToHalfBlocks(msg.img, videoW, videoH*2)
		}
		var cmd tea.Cmd
		if msg.qrText != "" {
			entry := formatQREntry(msg.qrText)
			m.webcamScanLog = append([]string{entry}, m.webcamScanLog...)
			if len(m.webcamScanLog) > 20 {
				m.webcamScanLog = m.webcamScanLog[:20]
			}
			m.addLog("success", "Scanned QR: "+truncate(msg.qrText, 80))
			cmd = waitForWebcamFrame(m.webcamFrameCh)
		} else {
			cmd = waitForWebcamFrame(m.webcamFrameCh)
		}
		return m, cmd, true

	case webcamErrMsg:
		if m.webcamActive {
			m.webcamErrStr = msg.err.Error()
			m.webcamActive = false
		}
		return m, nil, true
	}

	return m, nil, false
}

// formatQREntry tries to pretty-print text as JSON; falls back to the raw string.
func formatQREntry(text string) string {
	ts := time.Now().Format("15:04:05")
	var raw interface{}
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		pretty, err := json.MarshalIndent(raw, "  ", "  ")
		if err == nil {
			return fmt.Sprintf("[%s]\n  %s", ts, string(pretty))
		}
	}
	// Not JSON — wrap as a single-field object for consistent display
	wrapped := map[string]string{"data": text}
	pretty, err := json.MarshalIndent(wrapped, "  ", "  ")
	if err == nil {
		return fmt.Sprintf("[%s]\n  %s", ts, string(pretty))
	}
	return fmt.Sprintf("[%s] %s", ts, text)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// renderScanTxPanel renders the "Scan Signed Transaction" overlay panel.
func (m model) renderScanTxPanel() string {
	panelW := m.w - 4
	innerW := helpers.Max(0, panelW-2)

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)
	title := titleStyle.Render("◉ Scan Signed Transaction")

	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

	var videoBlock string
	if m.webcamErrStr != "" {
		errStyle := lipgloss.NewStyle().Foreground(styles.CError)
		videoBlock = errStyle.Render("Camera error: " + m.webcamErrStr)
	} else if m.webcamRendered == "" {
		videoBlock = muteStyle.Render("Initializing camera…")
	} else {
		videoBlock = m.webcamRendered
	}

	divider := lipgloss.NewStyle().Foreground(styles.CBorder).
		Render(strings.Repeat("─", innerW))

	logTitle := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("Decoded")
	var logLines string
	if len(m.webcamScanLog) == 0 {
		logLines = muteStyle.Render("No QR codes detected yet…")
	} else {
		logLines = strings.Join(m.webcamScanLog, "\n\n")
	}

	hint := muteStyle.Render("ESC to close")

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		videoBlock,
		"",
		divider,
		logTitle,
		logLines,
		"",
		hint,
	)

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder).
		Width(panelW).
		Padding(1, 2).
		Render(body)

	return appStyle.Render(lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		panel,
	))
}
