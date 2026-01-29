package home

import (
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/huh"
)

// TempSelection stores the home menu selection
var TempSelection string

// CreateForm creates the home menu form
func CreateForm() *huh.Form {
	TempSelection = ""

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Options(
					huh.NewOption("Account List", "accounts"),
					huh.NewOption("RPC Settings", "settings"),
					huh.NewOption("dApp Browser", "dapps"),
				).
				Title("Main Menu").
				Description("Select a view to navigate to").
				Value(&TempSelection),
		),
	).WithTheme(huh.ThemeCatppuccin())

	form.Init()
	return form
}

// Render renders the home view
func Render(form *huh.Form) string {
	if form != nil {
		return form.View()
	}
	return "Loading menu..."
}

// Nav returns the navigation bar for home view
func Nav(width int) string {
	left := strings.Join([]string{
		styles.Key("↑/↓") + " select",
		styles.Key("Enter") + " go",
		styles.Key("l") + " logger",
		styles.Key("Esc") + " quit",
	}, "   ")

	return styles.NavStyle.Width(width).Render(left)
}
