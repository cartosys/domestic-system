//go:build linux
// +build linux

package capture

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/blackjack/webcam"
)

const (
	defaultDevice = "/dev/video0"
	// V4L2 MJPEG four-character code (little-endian)
	mjpegFormat = webcam.PixelFormat(0x47504A4D)
)

type Camera struct {
	cam    *webcam.Webcam
	frames chan image.Image
	done   chan struct{}
}

func New(device string) (*Camera, error) {
	if device == "" {
		device = defaultDevice
	}
	cam, err := webcam.Open(device)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", device, err)
	}
	_, _, _, err = cam.SetImageFormat(mjpegFormat, 640, 480)
	if err != nil {
		cam.Close()
		return nil, fmt.Errorf("set MJPEG 640x480: %w", err)
	}
	return &Camera{
		cam:    cam,
		frames: make(chan image.Image, 2),
		done:   make(chan struct{}),
	}, nil
}

func (c *Camera) Start() error {
	if err := c.cam.StartStreaming(); err != nil {
		return fmt.Errorf("start streaming: %w", err)
	}
	go c.captureLoop()
	return nil
}

func (c *Camera) captureLoop() {
	defer close(c.frames)
	for {
		select {
		case <-c.done:
			return
		default:
		}
		if err := c.cam.WaitForFrame(1); err != nil {
			continue
		}
		data, err := c.cam.ReadFrame()
		if err != nil || len(data) == 0 {
			continue
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			continue
		}
		select {
		case c.frames <- img:
		default:
		}
	}
}

func (c *Camera) Frames() <-chan image.Image {
	return c.frames
}

func (c *Camera) Close() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.cam.StopStreaming()
	c.cam.Close()
}
