//go:build darwin
// +build darwin

package capture

import (
	"fmt"
	"image"
	"sync"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"

	_ "github.com/pion/mediadevices/pkg/driver/camera"
)

type Camera struct {
	stream mediadevices.MediaStream
	track  *mediadevices.VideoTrack
	reader video.Reader
	frames chan image.Image
	done   chan struct{}
	once   sync.Once
}

func New(device string) (*Camera, error) {
	stream, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open camera: %w", err)
	}
	tracks := stream.GetVideoTracks()
	if len(tracks) == 0 {
		for _, t := range stream.GetTracks() {
			_ = t.Close()
		}
		return nil, fmt.Errorf("no video tracks")
	}
	track, ok := tracks[0].(*mediadevices.VideoTrack)
	if !ok {
		for _, t := range stream.GetTracks() {
			_ = t.Close()
		}
		return nil, fmt.Errorf("invalid video track")
	}
	return &Camera{
		stream: stream,
		track:  track,
		reader: track.NewReader(false),
		frames: make(chan image.Image, 2),
		done:   make(chan struct{}),
	}, nil
}

func (c *Camera) Start() error {
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
		img, release, err := c.reader.Read()
		if err != nil {
			continue
		}
		if img == nil {
			release()
			continue
		}
		select {
		case c.frames <- img:
		default:
		}
		release()
	}
}

func (c *Camera) Frames() <-chan image.Image {
	return c.frames
}

func (c *Camera) Close() {
	c.once.Do(func() {
		close(c.done)
	})
	if c.track != nil {
		_ = c.track.Close()
	}
	if c.stream != nil {
		for _, t := range c.stream.GetTracks() {
			_ = t.Close()
		}
	}
}
