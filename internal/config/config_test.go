package config_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbOogway/openagentindex/internal/config"
)

const fullFile = `
[crawler]
db_path = "data/crawler.db"
seeds_path = "seeds.txt"
contact_url = "https://example.com/contact"

[tui]
db_path = "data/tui.db"

[api]
db_path = "data/api.db"
listen_addr = ":9000"
cors_origin = "http://localhost:3000"
`

var envVars = []string{
	config.EnvPath, "OAI_DB_PATH", "OAI_SEEDS_PATH", "OAI_CONTACT_URL", "OAI_LISTEN_ADDR", "OAI_CORS_ORIGIN",
}

// isolate runs the test in an empty directory without OAI_ variables. It sets env.
func isolate(t *testing.T, env map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	for _, v := range envVars {
		t.Setenv(v, "")
		if err := os.Unsetenv(v); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAPI(t *testing.T) {
	tests := []struct {
		name string
		file string
		env  map[string]string
		want config.API
	}{
		{
			name: "file only",
			file: fullFile,
			want: config.API{DBPath: "data/api.db", ListenAddr: ":9000", CORSOrigin: "http://localhost:3000"},
		},
		{
			name: "environment overrides the file",
			file: fullFile,
			env:  map[string]string{"OAI_LISTEN_ADDR": "127.0.0.1:8081", "OAI_DB_PATH": "/var/oai.db"},
			want: config.API{DBPath: "/var/oai.db", ListenAddr: "127.0.0.1:8081", CORSOrigin: "http://localhost:3000"},
		},
		{
			name: "empty variable clears an optional key",
			file: fullFile,
			env:  map[string]string{"OAI_CORS_ORIGIN": ""},
			want: config.API{DBPath: "data/api.db", ListenAddr: ":9000"},
		},
		{
			name: "environment only, no file",
			env:  map[string]string{"OAI_LISTEN_ADDR": ":8080", "OAI_DB_PATH": "oai.db"},
			want: config.API{DBPath: "oai.db", ListenAddr: ":8080"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := isolate(t, tt.env)
			if tt.file != "" {
				writeFile(t, filepath.Join(dir, config.DefaultPath), tt.file)
			}
			got, err := config.LoadAPI()
			if err != nil {
				t.Fatalf("LoadAPI: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadAPI = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadCrawler(t *testing.T) {
	dir := isolate(t, map[string]string{"OAI_SEEDS_PATH": "other-seeds.txt"})
	path := filepath.Join(dir, "custom.toml")
	writeFile(t, path, fullFile)
	t.Setenv(config.EnvPath, path)

	got, err := config.LoadCrawler()
	if err != nil {
		t.Fatalf("LoadCrawler: %v", err)
	}
	want := config.Crawler{DBPath: "data/crawler.db", SeedsPath: "other-seeds.txt", ContactURL: "https://example.com/contact"}
	if got != want {
		t.Errorf("LoadCrawler = %+v, want %+v", got, want)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		env      map[string]string
		want     error
		mentions []string
	}{
		{
			name:     "missing required keys",
			file:     "[api]\ncors_origin = \"x\"\n",
			want:     config.ErrMissing,
			mentions: []string{"api.db_path (OAI_DB_PATH)", "api.listen_addr (OAI_LISTEN_ADDR)"},
		},
		{
			name:     "empty variable clears a required key",
			file:     fullFile,
			env:      map[string]string{"OAI_LISTEN_ADDR": ""},
			want:     config.ErrMissing,
			mentions: []string{"api.listen_addr"},
		},
		{
			name:     "unknown key",
			file:     fullFile + "listen_port = 9000\n",
			want:     config.ErrUnknownKey,
			mentions: []string{"api.listen_port"},
		},
		{
			name: "OAI_CONFIG names a missing file",
			env:  map[string]string{config.EnvPath: "missing.toml"},
			want: fs.ErrNotExist,
		},
		{
			name:     "not TOML",
			file:     "[api\n",
			mentions: []string{config.DefaultPath},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := isolate(t, tt.env)
			if tt.file != "" {
				writeFile(t, filepath.Join(dir, config.DefaultPath), tt.file)
			}
			_, err := config.LoadAPI()
			if err == nil {
				t.Fatal("LoadAPI: no error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			for _, m := range tt.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("err = %q, want it to mention %q", err, m)
				}
			}
		})
	}
}

func TestLoadTUI(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.TUI
	}{
		{name: "file", want: config.TUI{DBPath: "data/tui.db"}},
		{name: "environment overrides the file", env: map[string]string{"OAI_DB_PATH": "/x.db"}, want: config.TUI{DBPath: "/x.db"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := isolate(t, tt.env)
			writeFile(t, filepath.Join(dir, config.DefaultPath), fullFile)
			got, err := config.LoadTUI()
			if err != nil {
				t.Fatalf("LoadTUI: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadTUI = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadTUIMissing(t *testing.T) {
	isolate(t, nil)
	if _, err := config.LoadTUI(); !errors.Is(err, config.ErrMissing) || !strings.Contains(err.Error(), "tui.db_path (OAI_DB_PATH)") {
		t.Errorf("err = %v, want ErrMissing for tui.db_path", err)
	}
}
