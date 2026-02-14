package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// -------------------- MAIN --------------------

func main() {
	m := newModel()
	p := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
