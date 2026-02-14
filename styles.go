package main

import (
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/lipgloss"
)

// -------------------- THEME (Lip Gloss) --------------------
// Styles now come from the styles package

var (
	cBg      = styles.CBg
	cPanel   = styles.CPanel
	cBorder  = styles.CBorder
	cMuted   = styles.CMuted
	cText    = styles.CText
	cAccent  = styles.CAccent
	cAccent2 = styles.CAccent2
	cWarn    = styles.CWarn

	appStyle       = styles.AppStyle
	titleStyle     = styles.TitleStyle
	panelStyle     = styles.PanelStyle
	navStyle       = styles.NavStyle
	hotkeyStyle    = lipgloss.NewStyle().Foreground(styles.CMuted)
	hotkeyKeyStyle = lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true)
	helpRightStyle = lipgloss.NewStyle().Foreground(styles.CMuted)
)
