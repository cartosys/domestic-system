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

// cmdEnableMouseAllMotion switches the terminal to all-motion reporting (hover events included).
func cmdEnableMouseAllMotion() tea.Cmd {
	return func() tea.Msg { return tea.EnableMouseAllMotion() }
}

// cmdEnableMouseCellMotion drops back to button-only reporting while a text input is active.
// This prevents motion escape sequences from leaking into focused input fields.
func cmdEnableMouseCellMotion() tea.Cmd {
	return func() tea.Msg { return tea.EnableMouseCellMotion() }
}
