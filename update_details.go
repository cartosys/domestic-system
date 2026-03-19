package main

import (
	"fmt"
	"strings"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) createNicknameForm() {
	// Find current wallet's nickname
	tempNicknameField = ""
	for _, w := range m.accounts {
		if strings.EqualFold(w.Address, m.details.Address) {
			tempNicknameField = w.Name
			break
		}
	}

	placeholderText := "Enter nickname"
	if tempNicknameField != "" {
		placeholderText = tempNicknameField
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Wallet Nickname").
				Description("Set a friendly name for this wallet").
				Value(&tempNicknameField).
				Placeholder(placeholderText),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) handleNicknameFormMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept ESC key to cancel form
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		m.nicknaming = false
		m.form = nil
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f

		// Check if form is completed
		if m.form.State == huh.StateCompleted {
			// Save nickname to wallet entry
			for i := range m.accounts {
				if strings.EqualFold(m.accounts[i].Address, m.details.Address) {
					oldName := m.accounts[i].Name
					m.accounts[i].Name = strings.TrimSpace(tempNicknameField)
					if oldName == "" && m.accounts[i].Name != "" {
						m.addLog("success", fmt.Sprintf("Set nickname `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
					} else if m.accounts[i].Name == "" {
						m.addLog("info", fmt.Sprintf("Cleared nickname for wallet `%s`", helpers.ShortenAddr(m.details.Address)))
					} else {
						m.addLog("success", fmt.Sprintf("Updated nickname to `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
					}
					break
				}
			}
			config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Logger: m.logEnabled})
			m.nicknaming = false
			m.form = nil
			return m, nil
		}

		// Check if form was aborted (ESC pressed)
		if m.form.State == huh.StateAborted {
			m.nicknaming = false
			m.form = nil
			return m, nil
		}
	}
	return m, cmd
}

func (m *model) handleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Don't handle keys if nicknaming form is active
	if !m.nicknaming {
		switch msg.String() {
		case "esc", "backspace":
			return m, m.navigateTo(config.PageWallets)

		case "r", "R":
			// refresh
			addr := common.HexToAddress(m.details.Address)
			m.loading = true
			m.addLog("info", fmt.Sprintf("Refreshing details for `%s`", helpers.ShortenAddr(m.details.Address)))
			return m, loadDetails(m.ethClient, addr, m.tokenWatch)

		case "n", "N":
			// nickname
			m.nicknaming = true
			m.createNicknameForm()
			return m, nil
		}
	}
	return m, nil
}
