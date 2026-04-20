package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/scrollbar"
	"charm-wallet-tui/webcam/render"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// panelBorderPaddingH is the total horizontal overhead of the scan panel:
// 1 (left border) + 2 (left padding) + 2 (right padding) + 1 (right border) = 6.
const panelBorderPaddingH = 6

// panelBorderPaddingV is the total vertical overhead:
// 1 (top border) + 1 (top padding) + 1 (bottom padding) + 1 (bottom border) = 4.
const panelBorderPaddingV = 4

// scanPanelDims returns (panelW, innerW, videoH, logH) for the current terminal size.
// innerW = content width, videoH = ▀ rows (already accounts for half-block doubling),
// logH = lines reserved for the log viewport.
func (m model) scanPanelDims() (panelW, innerW, videoH, logH int) {
	panelW = m.w - 4
	innerW = helpers.Max(1, panelW-panelBorderPaddingH)

	panelContentH := helpers.Max(1, m.h-4-panelBorderPaddingV)
	// Fixed UI rows inside the content area (title, blanks, divider, logTitle, hint).
	const fixedRows = 7
	remaining := helpers.Max(0, panelContentH-fixedRows)

	// Video gets ~60% of the remaining height, rounded down to even (half-block pairs).
	videoH = (remaining * 3 / 5) & ^1 // clear lowest bit → even
	if videoH < 2 {
		videoH = 2
	}
	logH = helpers.Max(1, remaining-videoH)
	return
}

// openScanTxDialog starts the webcam and transitions to dialogScanTx.
func (m *model) openScanTxDialog() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogScanTx
	m.webcamActive = true
	m.webcamRendered = ""
	m.webcamScanLog = nil
	m.webcamErrStr = ""
	_, _, _, logH := m.scanPanelDims()
	m.webcamLogVP = viewport.New(0, logH)
	m.webcamLogVP.SetContent(scanLogContent(m.webcamScanLog))
	m.webcamLogScroll = scrollbar.State{}
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
	case "up", "k":
		m.webcamLogVP.LineUp(1)
	case "down", "j":
		m.webcamLogVP.LineDown(1)
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
		_, innerW, videoH, logH := m.scanPanelDims()
		// videoW is innerW minus 1 col reserved for the scrollbar gutter
		videoW := helpers.Max(1, innerW-2)
		if videoW > 0 && videoH > 0 {
			// ImageToHalfBlocks takes pixel height = videoH*2
			m.webcamRendered = render.ImageToHalfBlocks(msg.img, videoW, videoH*2)
		}
		if msg.qrText != "" {
			entry := formatQREntry(msg.qrText)
			m.webcamScanLog = append([]string{entry}, m.webcamScanLog...)
			if len(m.webcamScanLog) > 50 {
				m.webcamScanLog = m.webcamScanLog[:50]
			}
			m.webcamLogVP.Height = logH
			m.webcamLogVP.SetContent(scanLogContent(m.webcamScanLog))
			m.webcamLogVP.GotoTop()
			m.addLog("success", "Scanned QR: "+truncate(msg.qrText, 80))
		} else {
			m.webcamLogVP.Height = logH
		}
		return m, waitForWebcamFrame(m.webcamFrameCh), true

	case webcamErrMsg:
		if m.webcamActive {
			m.webcamErrStr = msg.err.Error()
			m.webcamActive = false
		}
		return m, nil, true
	}

	return m, nil, false
}

// scanLogContent builds the string rendered inside the log viewport.
func scanLogContent(entries []string) string {
	if len(entries) == 0 {
		return lipgloss.NewStyle().Foreground(styles.CMuted).Render("No QR codes detected yet…")
	}
	entryStyle := lipgloss.NewStyle().Foreground(styles.CAccent2)
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(entryStyle.Render(e))
	}
	return b.String()
}

// formatQREntry pretty-prints a decoded QR payload as JSON.
func formatQREntry(text string) string {
	ts := time.Now().Format("15:04:05")
	var raw interface{}
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		if pretty, err := json.MarshalIndent(raw, "  ", "  "); err == nil {
			return fmt.Sprintf("[%s]\n  %s", ts, string(pretty))
		}
	}
	wrapped := map[string]string{"data": text}
	if pretty, err := json.MarshalIndent(wrapped, "  ", "  "); err == nil {
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
	panelW, innerW, videoH, logH := m.scanPanelDims()

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)
	title := titleStyle.Render("◉ Scan Signed Transaction")

	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

	// Video block — width is innerW-2 to leave a scrollbar gutter column on the right.
	videoW := helpers.Max(1, innerW-2)
	var videoBlock string
	switch {
	case m.webcamErrStr != "":
		videoBlock = lipgloss.NewStyle().Foreground(styles.CError).Render("Camera error: " + m.webcamErrStr)
	case m.webcamRendered == "":
		videoBlock = muteStyle.Render("Initializing camera…")
	default:
		videoBlock = m.webcamRendered
	}
	_ = videoH // height already baked into m.webcamRendered via scanPanelDims
	_ = videoW

	divider := lipgloss.NewStyle().Foreground(styles.CBorder).
		Render(strings.Repeat("─", innerW))

	logTitle := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("Decoded")

	// Log viewport with scrollbar.
	m.webcamLogVP.Width = innerW - 2 // -2 for scrollbar gutter
	m.webcamLogVP.Height = logH
	vpContent := m.webcamLogVP.View()
	track := scrollbar.Track(logH, m.webcamLogVP.TotalLineCount(), m.webcamLogVP.YOffset)
	logWithBar := scrollbar.Decorate(vpContent, track)

	hint := muteStyle.Render("↑/↓ scroll log   ESC to close")

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		videoBlock,
		"",
		divider,
		logTitle,
		logWithBar,
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
