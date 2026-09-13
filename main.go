package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/earendil/gomusic/internal/config"
	"github.com/earendil/gomusic/internal/player"
	"github.com/earendil/gomusic/internal/ui"
)

func main() {
	cfg := config.Load()
	p := player.New(cfg.Volume)
	p.Init()
	defer p.Close()
	p.SetVolume(cfg.Volume)

	model := ui.New(cfg, p)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
