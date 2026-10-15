package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type Config struct {
	Index Index
	Chat  Chat
}

// Run shows the terminal client until the user quits or ctx ends.
func Run(ctx context.Context, cfg Config) error {
	_, err := tea.NewProgram(newModel(ctx, cfg), tea.WithContext(ctx)).Run()
	return err
}
