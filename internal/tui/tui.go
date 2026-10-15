package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type Config struct {
	Index Index
}

// Run shows the terminal client until the user quits or ctx ends.
func Run(ctx context.Context, cfg Config) error {
	_, err := tea.NewProgram(newSearch(ctx, cfg.Index), tea.WithContext(ctx)).Run()
	return err
}
