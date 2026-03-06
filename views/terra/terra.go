package terra

import (
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for Terra Nullius view
func Nav(width int) string {
	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("Tab") + " next",
		styles.Key("Enter") + " select",
		styles.Key("Esc") + " back",
		styles.Key("l") + " logger",
	}, "   ")
	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the Terra Nullius interface with three elements
func Render(
	width, height int,
	focusedField int,
	claimsCount string, claimsLoading bool,
	claimInput string, claimQuerying bool,
	claimResult *helpers.TerraClaimResult, claimResultErr string,
) string {
	containerWidth := helpers.Min(80, width-4)

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(containerWidth)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Align(lipgloss.Center).
		Width(containerWidth)

	title := titleStyle.Render("🏜️  Terra Nullius")
	subtitle := subtitleStyle.Render(helpers.ShortenAddr(helpers.TerraContractAddress) + "  [Mainnet]")

	// --- Element 0: Number of Claims (display-only, unselectable) ---
	countBoxStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CMuted)

	countLabel := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render("Number of Claims")

	var countDisplay string
	if claimsLoading {
		countDisplay = lipgloss.NewStyle().Foreground(styles.CMuted).Render("Loading…")
	} else if claimsCount != "" {
		countDisplay = lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render(claimsCount)
	} else {
		countDisplay = lipgloss.NewStyle().Foreground(styles.CMuted).Render("—")
	}

	countBox := countBoxStyle.Render(countLabel + "\n" + countDisplay)

	// --- Element 1: Claims query (selectable) ---
	claimsBoxStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder)

	claimsBoxFocusedStyle := claimsBoxStyle.Copy().
		BorderForeground(styles.CAccent).
		BorderStyle(lipgloss.ThickBorder())

	claimsLabel := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Render("Claims")

	inputFieldStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Width(containerWidth - 8)

	var inputDisplay string
	if focusedField == 1 {
		inputDisplay = inputFieldStyle.Render(claimInput + "█")
	} else {
		inputDisplay = inputFieldStyle.Render(claimInput)
	}

	var resultContent string
	if claimQuerying {
		resultContent = lipgloss.NewStyle().Foreground(styles.CMuted).Render("Querying…")
	} else if claimResultErr != "" {
		resultContent = lipgloss.NewStyle().Foreground(styles.CError).Render(claimResultErr)
	} else if claimResult != nil {
		claimantDisplay := lipgloss.NewStyle().Foreground(styles.CAccent2).Render(helpers.ShortenAddr(claimResult.Claimant))
		blockDisplay := lipgloss.NewStyle().Foreground(styles.CMuted).Render("Block " + claimResult.BlockNumber.String())
		msgDisplay := lipgloss.NewStyle().Foreground(styles.CText).Render(`"` + claimResult.Message + `"`)
		resultContent = claimantDisplay + "  " + blockDisplay + "\n" + msgDisplay
	}

	claimsContent := claimsLabel + "\n" + inputDisplay
	if resultContent != "" {
		claimsContent += "\n" + resultContent
	}

	var claimsBox string
	if focusedField == 1 {
		claimsBox = claimsBoxFocusedStyle.Render(claimsContent)
	} else {
		claimsBox = claimsBoxStyle.Render(claimsContent)
	}

	// --- Element 2: Claim write function (selectable) ---
	claimBoxStyle := lipgloss.NewStyle().
		Width(containerWidth - 4).
		Padding(0, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder)

	claimBoxFocusedStyle := claimBoxStyle.Copy().
		BorderForeground(styles.CAccent).
		BorderStyle(lipgloss.ThickBorder())

	claimLabel := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Render("Claim")

	claimHint := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Render("Press Enter to make a claim on the blockchain")

	var claimBox string
	if focusedField == 2 {
		claimBox = claimBoxFocusedStyle.Render(claimLabel + "\n" + claimHint)
	} else {
		claimBox = claimBoxStyle.Render(claimLabel + "\n" + claimHint)
	}

	infoText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Width(containerWidth).
		Align(lipgloss.Center).
		Render("↑/↓ navigate • Tab next • Enter select")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title, subtitle, "",
		countBox, "",
		claimsBox, "",
		claimBox, "",
		infoText,
	)

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)
}

// RenderClaimPopup renders the claim form as a centered full-screen overlay.
// inputView is the already-rendered textinput.Model view string.
func RenderClaimPopup(width, height int, inputView, inputErr string, formFocused int) string {
	const popupWidth = 58

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.CAccent2).
		Bold(true).
		Align(lipgloss.Center).
		Width(popupWidth - 8)

	promptStyle := lipgloss.NewStyle().
		Foreground(styles.CText).
		Width(popupWidth - 8)

	inputLabelStyle := lipgloss.NewStyle().
		Foreground(styles.CMuted)

	inputBoxStyle := lipgloss.NewStyle().
		Width(popupWidth - 12).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder)

	inputBoxFocusedStyle := inputBoxStyle.Copy().
		BorderForeground(styles.CAccent)

	var inputBox string
	if formFocused == 0 {
		inputBox = inputBoxFocusedStyle.Render(inputView)
	} else {
		inputBox = inputBoxStyle.Render(inputView)
	}

	var errDisplay string
	if inputErr != "" {
		errDisplay = lipgloss.NewStyle().
			Foreground(styles.CError).
			Render("⚠ " + inputErr)
	}

	submitActiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF7DB")).
		Background(styles.CError).
		Padding(0, 3).
		MarginRight(0).
		Underline(true)

	submitInactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFF7DB")).
		Background(lipgloss.Color("#888B7E")).
		Padding(0, 3)

	var submitBtn string
	if formFocused == 1 {
		submitBtn = submitActiveStyle.Render("Send Claim")
	} else {
		submitBtn = submitInactiveStyle.Render("Send Claim")
	}

	helpText := lipgloss.NewStyle().
		Foreground(styles.CMuted).
		Align(lipgloss.Center).
		Width(popupWidth - 8).
		Render("Tab/↑↓ toggle • Enter submit • Esc cancel")

	boxStyle := lipgloss.NewStyle().
		Width(popupWidth).
		Padding(1, 4).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(styles.CBorder).
		Background(styles.CPanel)

	var uiParts []string
	uiParts = append(uiParts, titleStyle.Render("🏜️  Terra Nullius"))
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, promptStyle.Render("Make a statement on the blockchain forever:"))
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, inputLabelStyle.Render("Message"))
	uiParts = append(uiParts, inputBox)
	if errDisplay != "" {
		uiParts = append(uiParts, errDisplay)
	}
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, submitBtn)
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, helpText)

	ui := lipgloss.JoinVertical(lipgloss.Center, uiParts...)
	box := boxStyle.Render(ui)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}
