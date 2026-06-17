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

// enableHoverAllMotion is the single kill switch for hover-via-all-motion
// (Phase C). All-motion mode (tea.EnableMouseAllMotion / \x1b[?1003h) fires an
// escape sequence on every cursor move; on some Linux terminals these
// sequences have arrived on stdin in a form Bubble Tea's parser didn't
// recognize in time, landing as a tea.KeyMsg containing raw SGR bytes (e.g.
// "[<35;238;1M") that a focused huh textinput would faithfully type. The
// fix here is to only run all-motion while no text input has focus (see the
// end-of-Update() toggle keyed off textInputActive() in update.go) — cell
// motion mode (mode 1002), which reliably reports press/release/drag and is
// used for scrollbar dragging and clicks, is unaffected either way. If
// real-terminal testing ever reproduces the corruption under this toggle,
// flip this to false: the two commands below become no-ops again exactly
// like before, and clicks/click-to-focus do not depend on motion-without-click
// so they're unaffected by the revert.
const enableHoverAllMotion = true

// cmdEnableMouseAllMotion requests all-motion mouse reporting so hover can be
// detected without a button held. No-op when enableHoverAllMotion is false.
func cmdEnableMouseAllMotion() tea.Cmd {
	if !enableHoverAllMotion {
		return nil
	}
	return tea.EnableMouseAllMotion
}

// cmdEnableMouseCellMotion requests cell-motion mouse reporting (press/
// release/drag only) — the safe mode while any text input has focus.
func cmdEnableMouseCellMotion() tea.Cmd {
	if !enableHoverAllMotion {
		return nil
	}
	return tea.EnableMouseCellMotion
}
