package main

import (
	"fmt"
	"strings"

	"charm-wallet-tui/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func (m *model) createAddRPCForm() {
	tempRPCFormName = ""
	tempRPCFormURL = ""

	nameField := huh.NewInput().
		Title("RPC Name").
		Description("A friendly name for this RPC endpoint").
		Value(&tempRPCFormName).
		Placeholder("My Infura Node")

	urlField := huh.NewInput().
		Title("RPC URL").
		Description("The complete RPC URL (https://...)").
		Value(&tempRPCFormURL).
		Placeholder("https://mainnet.infura.io/v3/...")

	m.formFields = []huh.Field{nameField, urlField}
	m.form = huh.NewForm(
		huh.NewGroup(nameField, urlField),
	).WithWidth(RPCFormPopupInnerWidth).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

func (m *model) createEditRPCForm(idx int) {
	if idx < 0 || idx >= len(m.rpcURLs) {
		return
	}

	rpc := m.rpcURLs[idx]
	tempRPCFormName = rpc.Name
	tempRPCFormURL = rpc.URL

	nameField := huh.NewInput().
		Title("RPC Name").
		Value(&tempRPCFormName).
		Placeholder("My Node")

	urlField := huh.NewInput().
		Title("RPC URL").
		Value(&tempRPCFormURL).
		Placeholder("https://...")

	m.formFields = []huh.Field{nameField, urlField}
	m.form = huh.NewForm(
		huh.NewGroup(nameField, urlField),
	).WithWidth(RPCFormPopupInnerWidth).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

func (m *model) handleSettingsFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept ESC key to cancel form
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.settingsMode = "list"
		m.form = nil
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f

		// Check if form is completed
		if m.form.State == huh.StateCompleted {
			if m.settingsMode == "add" {
				if tempRPCFormName != "" && tempRPCFormURL != "" {
					newRPC := config.RPCUrl{Name: tempRPCFormName, URL: tempRPCFormURL, Active: false}
					m.rpcURLs = append(m.rpcURLs, newRPC)
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
					m.logSuccess(fmt.Sprintf("Added RPC endpoint: `%s` (%s)", tempRPCFormName, tempRPCFormURL))
				}
			} else if m.settingsMode == "edit" {
				if m.selectedRPCIdx >= 0 && m.selectedRPCIdx < len(m.rpcURLs) {
					m.rpcURLs[m.selectedRPCIdx].Name = tempRPCFormName
					m.rpcURLs[m.selectedRPCIdx].URL = tempRPCFormURL
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
					m.logSuccess(fmt.Sprintf("Updated RPC endpoint: `%s`", tempRPCFormName))
				}
			}
			m.settingsMode = "list"
			m.form = nil
			// Return without the form's cmd to ensure we're back in list mode
			return m, nil
		}

		// Check if form was aborted (ESC pressed)
		if m.form.State == huh.StateAborted {
			m.settingsMode = "list"
			m.form = nil
			return m, nil
		}
	}
	return m, cmd
}

// confirmDeleteRPCYes deletes the RPC endpoint pending confirmation. Shared
// by the keyboard Enter-on-Yes path and the dialog's mouse-clickable Yes button.
func (m *model) confirmDeleteRPCYes() (tea.Model, tea.Cmd) {
	idx := m.deleteRPCDialogIdx
	deletedName := m.deleteRPCDialogName
	if idx >= 0 && idx < len(m.rpcURLs) {
		m.rpcURLs = append(m.rpcURLs[:idx], m.rpcURLs[idx+1:]...)
		if m.selectedRPCIdx >= len(m.rpcURLs) && m.selectedRPCIdx > 0 {
			m.selectedRPCIdx--
		}
		config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
		m.logWarn(fmt.Sprintf("Deleted RPC endpoint `%s`", deletedName))
	}
	m.activeDialog = dialogNone
	return m, nil
}

// confirmDeleteRPCNo cancels the pending RPC endpoint deletion.
func (m *model) confirmDeleteRPCNo() (tea.Model, tea.Cmd) {
	m.activeDialog = dialogNone
	return m, nil
}

func (m *model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeDialog == dialogDeleteRPC {
		switch msg.String() {
		case "left", "right", "tab":
			m.deleteRPCDialogYesSelected = !m.deleteRPCDialogYesSelected
			return m, nil
		case "enter":
			if m.deleteRPCDialogYesSelected {
				return m.confirmDeleteRPCYes()
			}
			return m.confirmDeleteRPCNo()
		case "esc":
			m.activeDialog = dialogNone
			return m, nil
		}
		return m, nil
	}
	// Only handle list mode controls here (form handled at top of Update)
	if m.settingsMode == "list" {
		switch msg.String() {
		case "esc":
			return m, m.navigateTo(config.PageWallets)

		case "a", "A":
			m.settingsMode = "add"
			m.createAddRPCForm()
			return m, nil

		case "e", "E":
			if len(m.rpcURLs) > 0 {
				m.settingsMode = "edit"
				m.createEditRPCForm(m.selectedRPCIdx)
			}
			return m, nil

		case "delete", "backspace":
			if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
				m.activeDialog = dialogDeleteRPC
				m.deleteRPCDialogYesSelected = true
				m.deleteRPCDialogIdx = m.selectedRPCIdx
				name := strings.TrimSpace(m.rpcURLs[m.selectedRPCIdx].Name)
				if name == "" {
					name = m.rpcURLs[m.selectedRPCIdx].URL
				}
				m.deleteRPCDialogName = name
			}
			return m, nil

		case "up", "k":
			if m.selectedRPCIdx > 0 {
				m.selectedRPCIdx--
			}
			return m, nil

		case "down", "j":
			if m.selectedRPCIdx < len(m.rpcURLs)-1 {
				m.selectedRPCIdx++
			}
			return m, nil

		case "enter", " ":
			// Set as active
			if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
				for i := range m.rpcURLs {
					m.rpcURLs[i].Active = (i == m.selectedRPCIdx)
				}
				m.rpcURL = m.rpcURLs[m.selectedRPCIdx].URL
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
				// Set connecting state and reconnect with new RPC
				m.rpcConnecting = true
				m.rpcConnected = false
				return m, connectRPC(m.rpcURL)
			}
			return m, nil
		}
	}
	return m, nil
}
