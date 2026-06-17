package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// uiRegionKind distinguishes buttons (click-to-activate) from inputs
// (click-to-focus) for styling purposes — both are clickable the same way.
type uiRegionKind uint8

const (
	uiRegionButton uiRegionKind = iota
	uiRegionInput
)

// uiRegion is a single frame's clickable/hoverable hit-test rectangle,
// registered by render functions (the same "render mutates hit-test
// coordinates" pattern already used for fields like poolInfoOKBtnX1) and
// consumed by dispatchRegionClick before any other mouse handling runs.
type uiRegion struct {
	ID      string
	Kind    uiRegionKind
	X1, Y1  int
	X2, Y2  int // exclusive
	OnClick func(m *model) (tea.Model, tea.Cmd)
}

// registerRegion appends a region for the current frame. View() clears
// m.uiRegions at the start of every render, so this is rebuilt every frame.
func (m *model) registerRegion(id string, kind uiRegionKind, x1, y1, x2, y2 int, onClick func(m *model) (tea.Model, tea.Cmd)) {
	m.uiRegions = append(m.uiRegions, uiRegion{ID: id, Kind: kind, X1: x1, Y1: y1, X2: x2, Y2: y2, OnClick: onClick})
}

// hitTestRegions returns the topmost registered region containing (x, y), if any.
func (m *model) hitTestRegions(x, y int) (uiRegion, bool) {
	for i := len(m.uiRegions) - 1; i >= 0; i-- {
		r := m.uiRegions[i]
		if y >= r.Y1 && y < r.Y2 && x >= r.X1 && x < r.X2 {
			return r, true
		}
	}
	return uiRegion{}, false
}

// updateRegionHover refreshes m.hoveredRegionID for the cursor at (x, y) and
// reports whether it changed.
func (m *model) updateRegionHover(x, y int) bool {
	var id string
	if r, ok := m.hitTestRegions(x, y); ok {
		id = r.ID
	}
	changed := id != m.hoveredRegionID
	m.hoveredRegionID = id
	return changed
}

// dispatchRegionClick is the single entry point for region-aware mouse
// handling, called at the very top of Update() — before textInputActive()
// would otherwise drop the message. A click landing on a registered region
// always wins regardless of current focus state; anything else falls through
// unhandled so the rest of the mouse pipeline (including the textInputActive
// safety guard) runs exactly as before.
func (m *model) dispatchRegionClick(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.Type {
	case tea.MouseLeft:
		if region, ok := m.hitTestRegions(msg.X, msg.Y); ok && region.OnClick != nil {
			updated, cmd := region.OnClick(m)
			return updated, cmd, true
		}
	case tea.MouseMotion:
		if m.updateRegionHover(msg.X, msg.Y) {
			return m, nil, true
		}
	}
	return m, nil, false
}

// focusHuhField moves a huh.Form's focus to the field at targetIdx within
// fields (the same field pointers the form was built with), by stepping via
// huh's exported NextField()/PrevField() — huh has no random-access focus
// API. fields must be in the same order the form's single group was built
// with. A known limitation: if a validation-error line shifts field rows
// between the frame a region was registered and the frame the click is
// processed, the click can target the wrong field by one row — low
// frequency, not worth engineering around here.
func focusHuhField(form *huh.Form, fields []huh.Field, targetIdx int) tea.Cmd {
	if form == nil || targetIdx < 0 || targetIdx >= len(fields) {
		return nil
	}
	focused := form.GetFocusedField()
	current := -1
	for i, f := range fields {
		if f == focused {
			current = i
			break
		}
	}
	if current == -1 || current == targetIdx {
		return nil
	}
	var cmds []tea.Cmd
	if targetIdx > current {
		for i := current; i < targetIdx; i++ {
			cmds = append(cmds, tea.Cmd(huh.NextField))
		}
	} else {
		for i := current; i > targetIdx; i-- {
			cmds = append(cmds, tea.Cmd(huh.PrevField))
		}
	}
	return tea.Sequence(cmds...)
}
