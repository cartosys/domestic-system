//go:build darwin

package capture

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os/exec"
	"strings"
	"sync"
)

var (
	jpegSOI = []byte{0xFF, 0xD8}
	jpegEOI = []byte{0xFF, 0xD9}
)

type Camera struct {
	cmd       *exec.Cmd
	stdout    io.ReadCloser
	stderrBuf bytes.Buffer
	frames    chan image.Image
	done      chan struct{}
	once      sync.Once
}

func New(device string) (*Camera, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found — install with: brew install ffmpeg")
	}
	avInput := "0:none"
	if device != "" {
		avInput = device
	}
	cmd := exec.Command("ffmpeg",
		"-f", "avfoundation",
		"-framerate", "30",
		"-video_size", "640x480",
		"-i", avInput,
		"-f", "mjpeg",
		"-q:v", "5",
		"-loglevel", "error",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe: %w", err)
	}
	cam := &Camera{
		cmd:    cmd,
		stdout: stdout,
		frames: make(chan image.Image, 2),
		done:   make(chan struct{}),
	}
	cmd.Stderr = &cam.stderrBuf
	return cam, nil
}

func (c *Camera) Start() error {
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	go c.captureLoop(bufio.NewReaderSize(c.stdout, 1<<20))
	return nil
}

func (c *Camera) captureLoop(r io.Reader) {
	defer func() {
		_ = c.cmd.Wait() // flush stderr before signalling done
		close(c.frames)
	}()
	buf := make([]byte, 0, 1<<20)
	tmp := make([]byte, 65536)
	for {
		select {
		case <-c.done:
			return
		default:
		}
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				start := bytes.Index(buf, jpegSOI)
				if start < 0 {
					buf = buf[:0]
					break
				}
				end := bytes.Index(buf[start+2:], jpegEOI)
				if end < 0 {
					if start > 0 {
						buf = buf[start:]
					}
					break
				}
				end = start + 2 + end + 2
				img, jerr := jpeg.Decode(bytes.NewReader(buf[start:end]))
				if jerr == nil && img != nil {
					select {
					case c.frames <- img:
					default:
					}
				}
				buf = buf[end:]
			}
		}
		if err != nil {
			return
		}
	}
}

// Err returns the ffmpeg stderr output after the process exits.
// Filters macOS AVFoundation NSLog noise that bypasses ffmpeg's loglevel.
func (c *Camera) Err() error {
	var lines []string
	for _, line := range strings.Split(c.stderrBuf.String(), "\n") {
		if strings.Contains(line, "NSCameraUseContinuityCameraDeviceType") {
			continue
		}
		if t := strings.TrimSpace(line); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

func (c *Camera) Frames() <-chan image.Image {
	return c.frames
}

func (c *Camera) Close() {
	c.once.Do(func() {
		close(c.done)
	})
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		// Wait is called by captureLoop's deferred func.
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
}
