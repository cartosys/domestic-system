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
	case "p", "P":
		return m.openPasteSignedTxDialog()
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
			m.logSuccess("Scanned QR: "+truncate(msg.qrText, 80))
		} else {
			m.webcamLogVP.Height = logH
		}
		return m, waitForWebcamFrame(m.webcamFrameCh), true

	case webcamErrMsg:
		if m.webcamActive {
			m.webcamErrStr = msg.err.Error()
			m.webcamActive = false
			// No camera / camera error during the "scan signed tx response"
			// flow: fall straight through to the paste-a-signed-tx form,
			// focused and ready — there's nothing useful to show otherwise.
			if m.activeDialog == dialogScanTx {
				newModel, cmd := m.openPasteSignedTxDialog()
				return newModel, cmd, true
			}
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
// Pointer receiver so it can update PanelTop/TrackCol for mouse hit-testing.
func (m *model) renderScanTxPanel() string {
	panelW, innerW, videoH, logH := m.scanPanelDims()

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent).
		Bold(true).
		Width(innerW).
		Align(lipgloss.Center)
	title := titleStyle.Render("◉ Scan Signed Transaction")

	muteStyle := lipgloss.NewStyle().Foreground(styles.CMuted)

	var videoBlock string
	switch {
	case m.webcamErrStr != "":
		videoBlock = lipgloss.NewStyle().Foreground(styles.CError).Render("Camera error: " + m.webcamErrStr)
	case m.webcamRendered == "":
		videoBlock = muteStyle.Render("Initializing camera…")
	default:
		videoBlock = m.webcamRendered
	}

	divider := lipgloss.NewStyle().Foreground(styles.CBorder).
		Render(strings.Repeat("─", innerW))

	logTitle := lipgloss.NewStyle().Foreground(styles.CAccent2).Bold(true).Render("Decoded")

	// Log viewport with scrollbar.
	m.webcamLogVP.Width = innerW - 2 // reserve 1 col for space + 1 for scrollbar track
	m.webcamLogVP.Height = logH
	vpContent := m.webcamLogVP.View()
	track := scrollbar.Track(logH, m.webcamLogVP.TotalLineCount(), m.webcamLogVP.YOffset)
	logWithBar := scrollbar.Decorate(vpContent, track)

	// "Paste a signed transaction" button — alternate input path to scanning,
	// always available; auto-triggered instead when the camera errors out.
	pasteBtnStyle := styles.ButtonNormal
	if m.hoveredRegionID == "scanTx.pasteButton" {
		pasteBtnStyle = styles.ButtonActive
	}
	pasteBtn := pasteBtnStyle.Render("Paste a signed transaction")

	// Compute scroll/button hit-test coordinates.
	// Panel is centered: left edge = (m.w - panelW) / 2, top = (m.h - panelOuterH) / 2.
	// Content rows: title(1) blank(1) video(videoH) blank(1) divider(1) logTitle(1)
	//               log(logH) blank(1) button(1) blank(1) hint(1) = 9 + videoH + logH
	panelOuterH := 13 + videoH + logH // 9 content rows + 4 border/padding overhead
	panelTopY := (m.h - panelOuterH) / 2
	if panelTopY < 0 {
		panelTopY = 0
	}
	// Log VP top = panelTopY + 1(border) + 1(padding) + 1(title) + 1(blank) + videoH + 1(blank) + 1(div) + 1(logTitle)
	m.webcamLogScroll.PanelTop = panelTopY + 7 + videoH
	panelLeftX := (m.w - panelW) / 2
	// Track col = panel left + 1(border) + 2(padding) + vpWidth + 1(space from Decorate)
	m.webcamLogScroll.TrackCol = panelLeftX + 1 + 2 + m.webcamLogVP.Width + 1

	// Button row = panelTop + 1(border) + 1(padding) + 1(title) + 1(blank) + videoH
	//            + 1(blank) + 1(divider) + 1(logTitle) + logH + 1(blank)
	pasteTxBtnY := panelTopY + 8 + videoH + logH
	pasteTxBtnX1 := panelLeftX + 1 + 2
	pasteTxBtnX2 := pasteTxBtnX1 + lipgloss.Width(pasteBtn)
	m.registerRegion("scanTx.pasteButton", uiRegionButton, pasteTxBtnX1, pasteTxBtnY, pasteTxBtnX2, pasteTxBtnY+1,
		func(m *model) (tea.Model, tea.Cmd) { return m.openPasteSignedTxDialog() })

	hint := muteStyle.Render("↑/↓ scroll log   p paste signed tx   ESC to close")

	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		videoBlock,
		"",
		divider,
		logTitle,
		logWithBar,
		"",
		pasteBtn,
		"",
		hint,
	)

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder).
		Width(panelW).
		Padding(1, 2).
		Render(body)

	return styles.AppStyle.Render(lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		panel,
	))
}
