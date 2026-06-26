// Package anim provides an animated scrambling spinner, ported from
// github.com/charmbracelet/crush/internal/ui/anim.
package anim

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

const (
	fps           = 20
	initialChar   = '.'
	labelGap      = " "
	labelGapWidth = 1

	ellipsisAnimSpeed = 8
	maxBirthSteps     = 20
	prerenderedFrames = 10

	defaultNumCyclingChars = 10
)

var (
	defaultGradColorA = color.RGBA{R: 0xff, G: 0, B: 0, A: 0xff}
	defaultGradColorB = color.RGBA{R: 0, G: 0, B: 0xff, A: 0xff}
	defaultLabelColor = color.RGBA{R: 0xcc, G: 0xcc, B: 0xcc, A: 0xff}
)

var (
	availableRunes = []rune("0123456789abcdefABCDEF~!@#$£€%^&*()+=_")
	ellipsisFrames = []string{".", "..", "...", ""}
)

var lastID atomic.Int64

func nextID() int {
	return int(lastID.Add(1))
}

type animCache struct {
	initialFrames  [][]string
	cyclingFrames  [][]string
	width          int
	labelWidth     int
	label          []string
	ellipsisFrames []string
}

var (
	animCacheMu  sync.RWMutex
	animCacheMap = map[string]*animCache{}
)

func settingsHash(opts Settings) string {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d-%s-%v-%v-%v-%t",
		opts.Size, opts.Label, opts.LabelColor, opts.GradColorA, opts.GradColorB, opts.CycleColors)
	return fmt.Sprintf("%x", h.Sum64())
}

func hash64(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// StepMsg is sent each animation tick to advance the spinner.
type StepMsg struct{ ID string }

// Settings configures the animation.
type Settings struct {
	ID          string
	Size        int
	Label       string
	LabelColor  color.Color
	GradColorA  color.Color
	GradColorB  color.Color
	CycleColors bool
	// NoScramble hides cycling chars, showing only label + ellipsis.
	NoScramble bool
}

// Anim is an animated scrambling spinner component.
type Anim struct {
	width            int
	cyclingCharWidth int
	label            []string
	labelWidth       int
	labelColor       color.Color
	birthSteps       []int
	initialFrames    [][]string
	initialized      atomic.Bool
	cyclingFrames    [][]string
	step             atomic.Int64
	framesSinceStart atomic.Int64
	ellipsisStep     atomic.Int64
	ellipsisFrames   []string
	id               string
}

// New creates a new Anim from the given settings.
func New(opts Settings) *Anim {
	a := &Anim{}
	if opts.Size < 1 {
		opts.Size = defaultNumCyclingChars
	}
	if colorIsUnset(opts.GradColorA) {
		opts.GradColorA = defaultGradColorA
	}
	if colorIsUnset(opts.GradColorB) {
		opts.GradColorB = defaultGradColorB
	}
	if colorIsUnset(opts.LabelColor) {
		opts.LabelColor = defaultLabelColor
	}

	if opts.ID != "" {
		a.id = opts.ID
	} else {
		a.id = fmt.Sprintf("%d", nextID())
	}
	if opts.NoScramble {
		a.cyclingCharWidth = 0
	} else {
		a.cyclingCharWidth = opts.Size
	}
	a.labelColor = opts.LabelColor

	if opts.NoScramble {
		a.initialized.Store(true)
	}

	cacheKey := settingsHash(opts)

	animCacheMu.RLock()
	cached, exists := animCacheMap[cacheKey]
	animCacheMu.RUnlock()

	if exists {
		a.width = cached.width
		a.labelWidth = cached.labelWidth
		a.label = append([]string(nil), cached.label...)
		a.ellipsisFrames = append([]string(nil), cached.ellipsisFrames...)
		a.initialFrames = cached.initialFrames
		a.cyclingFrames = cached.cyclingFrames
	} else {
		a.labelWidth = lipgloss.Width(opts.Label)

		a.width = a.cyclingCharWidth
		if opts.Label != "" {
			if a.cyclingCharWidth > 0 {
				a.width += labelGapWidth
			}
			a.width += lipgloss.Width(opts.Label)
		}

		a.renderLabel(opts.Label)

		var ramp []color.Color
		numFrames := prerenderedFrames
		if opts.CycleColors {
			ramp = makeGradientRamp(a.width*3, opts.GradColorA, opts.GradColorB, opts.GradColorA, opts.GradColorB)
			numFrames = a.width * 2
		} else {
			ramp = makeGradientRamp(a.width, opts.GradColorA, opts.GradColorB)
		}

		a.initialFrames = make([][]string, numFrames)
		offset := 0
		for i := range a.initialFrames {
			a.initialFrames[i] = make([]string, a.width+labelGapWidth+a.labelWidth)
			for j := range a.initialFrames[i] {
				if j+offset >= len(ramp) {
					continue
				}
				var c color.Color
				if j <= a.cyclingCharWidth {
					c = ramp[j+offset]
				} else {
					c = opts.LabelColor
				}
				a.initialFrames[i][j] = lipgloss.NewStyle().
					Foreground(toLipgloss(c)).
					Render(string(initialChar))
			}
			if opts.CycleColors {
				offset++
			}
		}

		seed := hash64(cacheKey)
		rng := rand.New(rand.NewPCG(seed, ^seed))
		a.cyclingFrames = make([][]string, numFrames)
		offset = 0
		for i := range a.cyclingFrames {
			a.cyclingFrames[i] = make([]string, a.width)
			for j := range a.cyclingFrames[i] {
				if j+offset >= len(ramp) {
					continue
				}
				r := availableRunes[rng.IntN(len(availableRunes))]
				a.cyclingFrames[i][j] = lipgloss.NewStyle().
					Foreground(toLipgloss(ramp[j+offset])).
					Render(string(r))
			}
			if opts.CycleColors {
				offset++
			}
		}

		labelSlice := make([]string, len(a.label))
		copy(labelSlice, a.label)
		ellipsisSlice := make([]string, len(a.ellipsisFrames))
		copy(ellipsisSlice, a.ellipsisFrames)

		newCache := &animCache{
			initialFrames:  a.initialFrames,
			cyclingFrames:  a.cyclingFrames,
			width:          a.width,
			labelWidth:     a.labelWidth,
			label:          labelSlice,
			ellipsisFrames: ellipsisSlice,
		}
		animCacheMu.Lock()
		animCacheMap[cacheKey] = newCache
		animCacheMu.Unlock()
	}

	birthSeed := hash64(a.id + "|" + cacheKey)
	birthRng := rand.New(rand.NewPCG(birthSeed, ^birthSeed))
	a.birthSteps = make([]int, a.width)
	for i := range a.birthSteps {
		a.birthSteps[i] = birthRng.IntN(maxBirthSteps)
	}

	return a
}

// SetLabel updates the label text.
func (a *Anim) SetLabel(newLabel string) {
	a.labelWidth = lipgloss.Width(newLabel)
	a.width = a.cyclingCharWidth
	if newLabel != "" {
		if a.cyclingCharWidth > 0 {
			a.width += labelGapWidth
		}
		a.width += a.labelWidth
	}
	a.renderLabel(newLabel)
}

func (a *Anim) renderLabel(label string) {
	if a.labelWidth > 0 {
		labelRunes := []rune(label)
		a.label = make([]string, len(labelRunes))
		for i, r := range labelRunes {
			a.label[i] = lipgloss.NewStyle().Foreground(toLipgloss(a.labelColor)).Render(string(r))
		}
		a.ellipsisFrames = make([]string, len(ellipsisFrames))
		for i, frame := range ellipsisFrames {
			a.ellipsisFrames[i] = lipgloss.NewStyle().Foreground(toLipgloss(a.labelColor)).Render(frame)
		}
	} else {
		a.label = nil
		a.ellipsisFrames = nil
	}
}

// Start returns the first animation tick command.
func (a *Anim) Start() tea.Cmd {
	return a.nextTick()
}

// Animate advances one step and returns the next tick command.
// It ignores StepMsgs not addressed to this instance.
func (a *Anim) Animate(msg StepMsg) tea.Cmd {
	if msg.ID != a.id {
		return nil
	}

	s := a.step.Add(1)
	if int(s) >= len(a.cyclingFrames) {
		a.step.Store(0)
	}

	frames := a.framesSinceStart.Add(1)
	if a.initialized.Load() && a.labelWidth > 0 {
		ellipsisStep := a.ellipsisStep.Add(1)
		if int(ellipsisStep) >= ellipsisAnimSpeed*len(ellipsisFrames) {
			a.ellipsisStep.Store(0)
		}
	} else if !a.initialized.Load() && int(frames) >= maxBirthSteps {
		a.initialized.Store(true)
	}
	return a.nextTick()
}

// Render returns the current animation frame as a string.
func (a *Anim) Render() string {
	var b strings.Builder
	s := int(a.step.Load())
	frames := int(a.framesSinceStart.Load())
	for i := range a.width {
		switch {
		case !a.initialized.Load() && i < len(a.birthSteps) && frames < a.birthSteps[i]:
			b.WriteString(a.initialFrames[s][i])
		case i < a.cyclingCharWidth:
			b.WriteString(a.cyclingFrames[s][i])
		case i == a.cyclingCharWidth && a.cyclingCharWidth > 0:
			b.WriteString(labelGap)
		default:
			offset := a.cyclingCharWidth
			if a.cyclingCharWidth > 0 {
				offset += labelGapWidth
			}
			idx := i - offset
			if idx >= 0 && idx < len(a.label) {
				b.WriteString(a.label[idx])
			}
		}
	}
	if a.initialized.Load() && a.labelWidth > 0 {
		ellipsisIdx := int(a.ellipsisStep.Load()) / ellipsisAnimSpeed
		if ellipsisIdx >= 0 && ellipsisIdx < len(a.ellipsisFrames) {
			b.WriteString(a.ellipsisFrames[ellipsisIdx])
		}
	}
	return b.String()
}

// View is an alias for Render, satisfying drop-in compatibility with
// the bubbles spinner interface used throughout the codebase.
func (a *Anim) View() string { return a.Render() }

func (a *Anim) nextTick() tea.Cmd {
	return tea.Tick(time.Second/time.Duration(fps), func(time.Time) tea.Msg {
		return StepMsg{ID: a.id}
	})
}

func makeGradientRamp(size int, stops ...color.Color) []color.Color {
	if len(stops) < 2 {
		return nil
	}
	points := make([]colorful.Color, len(stops))
	for i, k := range stops {
		points[i], _ = colorful.MakeColor(k)
	}

	numSegments := len(stops) - 1
	blended := make([]color.Color, 0, size)
	segmentSizes := make([]int, numSegments)
	baseSize := size / numSegments
	remainder := size % numSegments
	for i := range numSegments {
		segmentSizes[i] = baseSize
		if i < remainder {
			segmentSizes[i]++
		}
	}
	for i := range numSegments {
		c1 := points[i]
		c2 := points[i+1]
		segSize := segmentSizes[i]
		for j := range segSize {
			if segSize == 0 {
				continue
			}
			t := float64(j) / float64(segSize)
			blended = append(blended, c1.BlendHcl(c2, t))
		}
	}
	return blended
}

func colorIsUnset(c color.Color) bool {
	if c == nil {
		return true
	}
	_, _, _, a := c.RGBA()
	return a == 0
}

// toLipgloss converts a standard color.Color to a lipgloss.Color hex string.
func toLipgloss(c color.Color) lipgloss.Color {
	cf, _ := colorful.MakeColor(c)
	return lipgloss.Color(cf.Hex())
}
