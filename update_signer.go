package main

import (
	"charm-wallet-tui/config"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleSignerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, m.navigateTo(config.PageWallets)

	case "up", "k":
		if m.signerKeyIdx > 0 {
			m.signerKeyIdx--
		}

	case "down", "j":
		if m.signerKeyIdx < len(m.signerKeys)-1 {
			m.signerKeyIdx++
		}

	case "s", "S":
		// Open webcam in signer mode
		m.signerScanMode = true
		m.signerDecoded = nil
		m.signerResult = nil
		m.signerSignErr = ""
		return m.openScanTxDialog()

	case "a", "A":
		// Re-load keys (bootstraps the file, shows any newly added key)
		return m, loadSignerKeys()

	case "c", "C":
		// Clear decoded tx and result
		m.signerDecoded = nil
		m.signerResult = nil
		m.signerSignErr = ""
	}

	return m, nil
}
