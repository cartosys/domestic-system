package main

import (
	"fmt"
	"image"
	"time"

	"charm-wallet-tui/webcam/capture"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/makiuchi-d/gozxing"
	gozxingqr "github.com/makiuchi-d/gozxing/qrcode"
)

// openWebcamCmd opens the camera and starts streaming. Returns webcamReadyMsg or webcamErrMsg.
func openWebcamCmd() tea.Msg {
	cam, err := capture.New("")
	if err != nil {
		return webcamErrMsg{err}
	}
	if err := cam.Start(); err != nil {
		cam.Close()
		return webcamErrMsg{err}
	}
	return webcamReadyMsg{cam: cam, ch: cam.Frames()}
}

// waitForWebcamFrame blocks until the next camera frame arrives, then decodes any QR code in it.
func waitForWebcamFrame(ch <-chan image.Image) tea.Cmd {
	return func() tea.Msg {
		img, ok := <-ch
		if !ok {
			return webcamErrMsg{fmt.Errorf("webcam stream closed")}
		}
		return webcamFrameMsg{img: img, qrText: decodeQR(img)}
	}
}

// decodeQR attempts to extract a QR code string from img. Returns "" on failure.
func decodeQR(img image.Image) string {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return ""
	}
	result, err := gozxingqr.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return ""
	}
	return result.String()
}

// animateQRTick fires txQRAnimTickMsg after 400 ms, advancing the animated QR display.
func animateQRTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(_ time.Time) tea.Msg {
		return txQRAnimTickMsg{}
	})
}

// cmdEnableMouseAllMotion is intentionally a no-op.
//
// All-motion mode (tea.EnableMouseAllMotion / \x1b[?1003h) fires an escape
// sequence on every cursor move. On Linux, these sequences arrive on stdin
// before Bubble Tea's parser classifies them, so they come through as
// tea.KeyMsg containing the raw SGR bytes (e.g. "[<35;238;1M"). When a huh
// textinput has focus it faithfully types those characters into the field.
//
// cell-motion mode (tea.WithMouseCellMotion / \x1b[?1002h) — the program
// default — reports button press, release, and motion while a button is held.
// This is sufficient for scrollbar dragging and click handling. Hover
// detection without a button pressed (sendButtonHovered) is disabled as a
// consequence, but that was purely cosmetic.
func cmdEnableMouseAllMotion() tea.Cmd { return nil }

// cmdEnableMouseCellMotion is kept for call-site compatibility but is now a
// no-op because the program stays in cell-motion mode for its entire lifetime.
func cmdEnableMouseCellMotion() tea.Cmd { return nil }
