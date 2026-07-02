package main

import (
	"fmt"
	"strings"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/views/watchedtokens"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) createAddTokenForm() {
	tempTokenFormAddr = ""
	m.tokenFormButtonFocused = false
	m.tokenFormError = ""

	addrField := huh.NewInput().
		Title("Token Contract Address").
		Description("ERC-20 contract address — symbol and decimals are read on-chain").
		Value(&tempTokenFormAddr).
		Placeholder("0x...")

	m.tokenFormFields = []huh.Field{addrField}
	m.tokenForm = huh.NewForm(
		huh.NewGroup(addrField),
	).WithWidth(RPCFormPopupInnerWidth).WithTheme(huh.ThemeCatppuccin())

	m.tokenForm.Init()
}

func (m *model) createEditTokenForm(idx int) {
	if idx < 0 || idx >= len(m.tokenWatch) {
		return
	}
	m.editingTokenIdx = idx
	tempTokenFormAddr = m.tokenWatch[idx].Address.Hex()
	m.tokenFormButtonFocused = false
	m.tokenFormError = ""

	addrField := huh.NewInput().
		Title("Token Contract Address").
		Value(&tempTokenFormAddr).
		Placeholder("0x...")

	m.tokenFormFields = []huh.Field{addrField}
	m.tokenForm = huh.NewForm(
		huh.NewGroup(addrField),
	).WithWidth(RPCFormPopupInnerWidth).WithTheme(huh.ThemeCatppuccin())

	m.tokenForm.Init()
}

// submitTokenForm validates the pasted address and kicks off an on-chain
// symbol()/name()/decimals()/totalSupply() lookup; the form stays open
// (with a spinner) until handleTokenMetadataMsg resolves it.
func (m *model) submitTokenForm() (tea.Model, tea.Cmd) {
	addr := strings.TrimSpace(tempTokenFormAddr)
	if !common.IsHexAddress(addr) {
		m.tokenFormError = "Not a valid address"
		return m, nil
	}
	m.tokenLookupActive = true
	m.tokenFormError = ""
	return m, fetchTokenMetadata(m.ethClient, common.HexToAddress(addr))
}

// handleTokenMetadataMsg applies the result of an in-flight
// symbol()/name()/decimals()/totalSupply() lookup triggered by submitTokenForm.
func (m *model) handleTokenMetadataMsg(msg tokenMetadataMsg) (tea.Model, tea.Cmd) {
	m.tokenLookupActive = false
	if msg.err != nil {
		m.tokenFormError = "Lookup failed: " + msg.err.Error()
		return m, nil
	}

	newToken := rpc.WatchedToken{Symbol: msg.symbol, Name: msg.name, Decimals: msg.decimals, Address: msg.address, TotalSupply: msg.totalSupply}
	if m.tokenFormMode == "add" {
		m.tokenWatch = append(m.tokenWatch, newToken)
		m.logSuccess(fmt.Sprintf("Added watched token: `%s` (%s)", msg.symbol, msg.address.Hex()))
	} else if m.tokenFormMode == "edit" {
		if m.editingTokenIdx >= 0 && m.editingTokenIdx < len(m.tokenWatch) {
			m.tokenWatch[m.editingTokenIdx] = newToken
			m.logSuccess(fmt.Sprintf("Updated watched token: `%s`", msg.symbol))
		}
	}
	config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled, WatchedTokens: tokenWatchToConfigList(m.tokenWatch)})

	m.tokenFormMode = "list"
	m.tokenForm = nil
	m.tokenFormButtonFocused = false
	return m, m.loadSelectedWalletDetailsFresh()
}

func (m *model) handleWatchedTokensFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			m.tokenFormMode = "list"
			m.tokenForm = nil
			m.tokenFormButtonFocused = false
			return m, nil
		}

		// Read the clipboard ourselves, synchronously, and feed it in as
		// typed runes rather than letting huh's async Paste cmd run: in a
		// multi-field group, huh.Group.Update (v0.7.0) calls the focused
		// field's Update twice for any non-KeyMsg message — once via its
		// broadcast-to-all-fields branch, once via its focused-field branch
		// — which double-inserts the pasted text.
		if keyMsg.String() == "ctrl+v" && !m.tokenFormButtonFocused {
			if text, err := clipboard.ReadAll(); err == nil && text != "" {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
			} else {
				return m, nil
			}
		}

		lastField := m.tokenFormFields[len(m.tokenFormFields)-1]

		if m.tokenFormButtonFocused {
			switch keyMsg.String() {
			case "enter", " ":
				return m.submitTokenForm()
			case "tab":
				m.tokenFormButtonFocused = false
				return m, focusHuhField(m.tokenForm, m.tokenFormFields, 0)
			case "shift+tab":
				m.tokenFormButtonFocused = false
				return m, focusHuhField(m.tokenForm, m.tokenFormFields, len(m.tokenFormFields)-1)
			}
			return m, nil
		}

		if keyMsg.String() == "tab" && m.tokenForm.GetFocusedField() == lastField {
			m.tokenFormButtonFocused = true
			return m, lastField.Blur()
		}
	}

	form, cmd := m.tokenForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.tokenForm = f

		if m.tokenForm.State == huh.StateCompleted {
			return m.submitTokenForm()
		}
		if m.tokenForm.State == huh.StateAborted {
			m.tokenFormMode = "list"
			m.tokenForm = nil
			m.tokenFormButtonFocused = false
			return m, nil
		}
	}
	return m, cmd
}

// confirmDeleteTokenYes deletes the watched token pending confirmation.
func (m *model) confirmDeleteTokenYes() (tea.Model, tea.Cmd) {
	idx := m.deleteTokenDialogIdx
	deletedName := m.deleteTokenDialogName
	if idx >= 0 && idx < len(m.tokenWatch) {
		m.tokenWatch = append(m.tokenWatch[:idx], m.tokenWatch[idx+1:]...)
		if m.selectedTokenIdx >= len(m.tokenWatch) && m.selectedTokenIdx > 0 {
			m.selectedTokenIdx--
		}
		config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled, WatchedTokens: tokenWatchToConfigList(m.tokenWatch)})
		m.logWarn(fmt.Sprintf("Removed watched token `%s`", deletedName))
	}
	m.activeDialog = dialogNone
	return m, nil
}

func (m *model) confirmDeleteTokenNo() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	return m, nil
}

// filteredOndoTokens returns helpers.OndoGMTokenList entries whose symbol or
// name contains m.ondoPickerFilter (case-insensitive substring match).
func (m *model) filteredOndoTokens() []helpers.OndoToken {
	q := strings.ToLower(strings.TrimSpace(m.ondoPickerFilter))
	if q == "" {
		return helpers.OndoGMTokenList
	}
	var out []helpers.OndoToken
	for _, t := range helpers.OndoGMTokenList {
		if strings.Contains(strings.ToLower(t.Symbol), q) || strings.Contains(strings.ToLower(t.Name), q) {
			out = append(out, t)
		}
	}
	return out
}

// handleOndoPickerKey drives the Ondo Global Markets token picker
// (dialogOndoPicker). Selecting an entry autofills the existing add-token
// form's address field and submits it — symbol/decimals are still verified
// on-chain by submitTokenForm/fetchTokenMetadata, exactly like a manually
// pasted address.
func (m *model) handleOndoPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredOndoTokens()
	switch msg.String() {
	case "esc":
		m.activeDialog = dialogNone
		m.ondoPickerFilter = ""
		m.ondoPickerIdx = 0
		return m, nil
	case "up", "k":
		if m.ondoPickerIdx > 0 {
			m.ondoPickerIdx--
		}
		return m, nil
	case "down", "j":
		if m.ondoPickerIdx < len(filtered)-1 {
			m.ondoPickerIdx++
		}
		return m, nil
	case "backspace":
		if len(m.ondoPickerFilter) > 0 {
			m.ondoPickerFilter = m.ondoPickerFilter[:len(m.ondoPickerFilter)-1]
			m.ondoPickerIdx = 0
		}
		return m, nil
	case "enter":
		if m.ondoPickerIdx < 0 || m.ondoPickerIdx >= len(filtered) {
			return m, nil
		}
		selected := filtered[m.ondoPickerIdx]
		m.activeDialog = dialogNone
		m.ondoPickerFilter = ""
		m.ondoPickerIdx = 0
		m.tokenFormMode = "add"
		m.createAddTokenForm()
		tempTokenFormAddr = selected.Address.Hex()
		return m.submitTokenForm()
	}
	if len(msg.Runes) > 0 {
		m.ondoPickerFilter += string(msg.Runes)
		m.ondoPickerIdx = 0
	}
	return m, nil
}

func (m *model) handleWatchedTokensKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogOndoPicker {
		return m.handleOndoPickerKey(msg)
	}

	if m.activeDialog == dialogDeleteToken {
		switch msg.String() {
		case "left", "right", "tab":
			m.deleteTokenDialogYesSelected = !m.deleteTokenDialogYesSelected
			return m, nil
		case "enter":
			if m.deleteTokenDialogYesSelected {
				return m.confirmDeleteTokenYes()
			}
			return m.confirmDeleteTokenNo()
		case "esc":
			m.activeDialog = dialogNone
			return m, nil
		}
		return m, nil
	}

	// While an on-chain symbol()/decimals() lookup is in flight, the form is
	// frozen (its messages now bypass handleWatchedTokensFormMsg — see the
	// !m.tokenLookupActive guard in update.go — so tokenMetadataMsg and the
	// spinner tick can actually reach their handlers). Esc still cancels.
	if m.tokenLookupActive {
		if msg.String() == "esc" {
			m.tokenLookupActive = false
			m.tokenFormMode = "list"
			m.tokenForm = nil
			m.tokenFormButtonFocused = false
			return m, nil
		}
		return m, nil
	}

	sorted := sortedWatchedTokens(m.tokenWatch, m.details)

	if m.tokenFormMode == "list" {
		switch msg.String() {
		case "esc":
			return m, m.navigateTo(config.PageWallets)

		case "a", "A":
			m.tokenFormMode = "add"
			m.createAddTokenForm()
			return m, nil

		case "o", "O":
			m.activeDialog = dialogOndoPicker
			m.ondoPickerFilter = ""
			m.ondoPickerIdx = 0
			return m, nil

		case "e", "E":
			if len(sorted) > 0 && m.selectedTokenIdx < len(sorted) {
				idx := tokenWatchIndex(m.tokenWatch, sorted[m.selectedTokenIdx].Address)
				m.tokenFormMode = "edit"
				m.createEditTokenForm(idx)
			}
			return m, nil

		case "delete", "backspace":
			if len(sorted) > 0 && m.selectedTokenIdx < len(sorted) {
				token := sorted[m.selectedTokenIdx]
				idx := tokenWatchIndex(m.tokenWatch, token.Address)
				m.activeDialog = dialogDeleteToken
				m.deleteTokenDialogYesSelected = true
				m.deleteTokenDialogIdx = idx
				m.deleteTokenDialogName = token.Symbol
			}
			return m, nil

		case "up", "k":
			if m.selectedTokenIdx > 0 {
				m.selectedTokenIdx--
				m.ensureTokenRowVisible()
			}
			return m, nil

		case "down", "j":
			if m.selectedTokenIdx < len(sorted)-1 {
				m.selectedTokenIdx++
				m.ensureTokenRowVisible()
			}
			return m, nil
		}
	}
	return m, nil
}

// ensureTokenRowVisible scrolls m.tokenListViewport so the row at
// m.selectedTokenIdx is visible, using the same line layout watchedtokens.Render
// produces (HeaderLines before the first row, RowHeight lines per row).
func (m *model) ensureTokenRowVisible() {
	top := watchedtokens.HeaderLines + m.selectedTokenIdx*watchedtokens.RowHeight
	bottom := top + watchedtokens.RowHeight - 1
	vp := &m.tokenListViewport
	if vp.Height <= 0 {
		return
	}
	if top < vp.YOffset {
		vp.SetYOffset(top)
	} else if bottom >= vp.YOffset+vp.Height {
		vp.SetYOffset(bottom - vp.Height + 1)
	}
}

// tokenWatchIndex returns the index of addr within watch, or -1 if not found.
func tokenWatchIndex(watch []rpc.WatchedToken, addr common.Address) int {
	for i, t := range watch {
		if t.Address == addr {
			return i
		}
	}
	return -1
}
