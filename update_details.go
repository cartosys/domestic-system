package main

import (
	"fmt"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

func (m *model) handleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		return m, m.navigateTo(config.PageWallets)

	case "r", "R":
		// refresh
		addr := common.HexToAddress(m.details.Address)
		m.loading = true
		m.logInfo(fmt.Sprintf("Refreshing details for `%s`", helpers.ShortenAddr(m.details.Address)))
		return m, loadDetails(m.ethClient, addr, m.tokenWatch)
	}
	return m, nil
}
