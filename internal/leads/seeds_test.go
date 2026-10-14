package leads_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sbOogway/openagentindex/internal/leads"
)

func TestSeedFileHosts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "one host on each line",
			content: "a.example\nb.example\n",
			want:    []string{"a.example", "b.example"},
		},
		{
			name:    "empty lines and comments",
			content: "# seeds\n\na.example\n  # indented comment\n\nb.example",
			want:    []string{"a.example", "b.example"},
		},
		{
			name:    "spaces and upper case",
			content: "  A.Example  \r\n\tb.example:8443\t\n",
			want:    []string{"a.example", "b.example:8443"},
		},
		{
			name:    "duplicates keep the first position",
			content: "a.example\nb.example\nA.EXAMPLE\n",
			want:    []string{"a.example", "b.example"},
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "seeds.txt")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := leads.SeedFile{Path: path}.Hosts(context.Background())
			if err != nil {
				t.Fatalf("Hosts: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Hosts = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeedFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")
	_, err := leads.SeedFile{Path: path}.Hosts(context.Background())
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Hosts error = %v, want fs.ErrNotExist", err)
	}
}
