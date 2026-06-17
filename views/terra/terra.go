package terra

import (
	"fmt"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/styles"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Nav returns the navigation bar for Terra Nullius view
func Nav(width int, indexerActive bool) string {
	var iItem string
	if indexerActive {
		iKey := lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true).Render("i")
		iLabel := lipgloss.NewStyle().Foreground(styles.CAccent).Render("indexer")
		iItem = iKey + " " + iLabel
	} else {
		iItem = styles.Key("i") + " indexer"
	}

	left := strings.Join([]string{
		styles.Key("↑/↓") + " navigate",
		styles.Key("Tab") + " next",
		styles.Key("Enter") + " select",
		styles.Key("Esc") + " back",
		styles.Key("l") + " logger",
		iItem,
	}, "   ")
	return styles.NavStyle.Width(width).Render(left)
}

// Render renders the Terra Nullius interface with three elements
func Render(
	width, height int,
	focusedField int,
	description string,
	claimsCount string, claimsLoading bool,
	claimInput string, claimQuerying bool,
	queriedIdx string,
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

	title := titleStyle.Render("🌵  Terra Nullius")
	subtitle := subtitleStyle.Render(helpers.ShortenAddr(helpers.TerraContractAddress) + "  [Mainnet]")

	var descLines []string
	if description != "" {
		descHeadStyle := lipgloss.NewStyle().
			Foreground(styles.CAccent2).
			Width(containerWidth).
			Align(lipgloss.Center)
		descBodyStyle := lipgloss.NewStyle().
			Foreground(styles.CMuted).
			Width(containerWidth).
			Align(lipgloss.Center)
		paragraphs := strings.Split(description, "\n\n")
		for i, p := range paragraphs {
			if i == 0 {
				descLines = append(descLines, descHeadStyle.Render(p))
			} else {
				descLines = append(descLines, descBodyStyle.Render(p))
			}
		}
	}

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
	claimsBoxStyle := styles.CardNormal.Width(containerWidth - 4)
	claimsBoxFocusedStyle := styles.CardFocused.Width(containerWidth - 4)

	claimsLabel := lipgloss.NewStyle().
		Foreground(styles.CText).
		Bold(true).
		Render("Read Claims")

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
		etherscanURL := fmt.Sprintf("https://etherscan.io/address/%s", claimResult.Claimant)
		rainbowAddr := helpers.FadeString(claimResult.Claimant, "#F25D94", "#79C0FF")
		claimantDisplay := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", etherscanURL, rainbowAddr)
		rainbowBlock := helpers.FadeString(claimResult.BlockNumber.String(), "#F25D94", "#79C0FF")
		muted := lipgloss.NewStyle().Foreground(styles.CMuted)
		idxDisplay := muted.Render("#" + queriedIdx + " sent by")
		atDisplay := muted.Render("at Block")
		msgDisplay := lipgloss.NewStyle().Foreground(styles.CText).Width(containerWidth - 8).Align(lipgloss.Center).Render(`"` + claimResult.Message + `"`)
		resultContent = idxDisplay + " " + claimantDisplay + " " + atDisplay + " " + rainbowBlock + "\n" + msgDisplay
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
	claimBoxStyle := styles.CardNormal.Width(containerWidth - 4)
	claimBoxFocusedStyle := styles.CardFocused.Width(containerWidth - 4)

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

	headerParts := []string{title, subtitle}
	if len(descLines) > 0 {
		headerParts = append(headerParts, "")
		headerParts = append(headerParts, descLines...)
	}
	headerParts = append(headerParts, "")

	contentParts := append(headerParts, countBox, "", claimsBox, "", claimBox, "", infoText)
	content := lipgloss.JoinVertical(lipgloss.Center, contentParts...)

	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)
}

// ClaimPopupGeometry reports screen-space hit-test rectangles for the
// clickable elements of RenderClaimPopup, computed from the same
// JoinVertical(Center, ...) layout actually rendered — measured directly
// rather than re-derived from style internals, so it stays correct if the
// styles change.
type ClaimPopupGeometry struct {
	InputY, InputX1, InputX2   int
	ButtonY, ButtonX1, ButtonX2 int
}

// RenderClaimPopup renders the claim form as a centered full-screen overlay.
// inputView is the already-rendered textinput.Model view string. Returns the
// rendered popup plus hit-test geometry for the input box and Send Claim button.
func RenderClaimPopup(width, height int, inputView, inputErr string, formFocused int) (string, ClaimPopupGeometry) {
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

	inputBoxStyle := styles.CardNormal.Width(popupWidth - 12).Padding(0, 1)
	inputBoxFocusedStyle := styles.CardFocused.Width(popupWidth - 12).Padding(0, 1)

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

	submitActiveStyle := styles.ButtonActive
	submitInactiveStyle := styles.ButtonNormal

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
	uiParts = append(uiParts, titleStyle.Render("🌵  Terra Nullius"))
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, promptStyle.Render("Make a statement on the blockchain forever:"))
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, inputLabelStyle.Render("Message"))
	inputBoxIdx := len(uiParts)
	uiParts = append(uiParts, inputBox)
	if errDisplay != "" {
		uiParts = append(uiParts, errDisplay)
	}
	uiParts = append(uiParts, "")
	submitBtnIdx := len(uiParts)
	uiParts = append(uiParts, submitBtn)
	uiParts = append(uiParts, "")
	uiParts = append(uiParts, helpText)

	ui := lipgloss.JoinVertical(lipgloss.Center, uiParts...)
	box := boxStyle.Render(ui)

	// Replicate JoinVertical(Center, ...)'s own centering math (pad every line
	// to the widest part, then center each within that) to compute screen-space
	// hit-test rectangles, rather than re-deriving offsets from style internals.
	maxW := 0
	for _, p := range uiParts {
		if w := lipgloss.Width(p); w > maxW {
			maxW = w
		}
	}
	rowY := func(idx int) int {
		y := 0
		for i := 0; i < idx; i++ {
			y += lipgloss.Height(uiParts[i])
		}
		return y
	}
	colX := func(s string) int {
		return (maxW - lipgloss.Width(s)) / 2
	}

	boxW := lipgloss.Width(box)
	boxH := lipgloss.Height(box)
	startX := (width - boxW) / 2
	startY := (height - boxH) / 2
	contentLeft := startX + 1 + 4 // border(1) + padding-left(4)
	contentTop := startY + 1 + 1  // border(1) + padding-top(1)

	geo := ClaimPopupGeometry{
		InputY:  contentTop + rowY(inputBoxIdx),
		InputX1: contentLeft + colX(inputBox),
		InputX2: contentLeft + colX(inputBox) + lipgloss.Width(inputBox),
		ButtonY: contentTop + rowY(submitBtnIdx),
		ButtonX1: contentLeft + colX(submitBtn),
		ButtonX2: contentLeft + colX(submitBtn) + lipgloss.Width(submitBtn),
	}

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		box,
	), geo
}
