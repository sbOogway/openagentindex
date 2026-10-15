package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	EnvPath     = "OAI_CONFIG"
	DefaultPath = "oai.toml"
)

var (
	ErrMissing    = errors.New("missing configuration")
	ErrUnknownKey = errors.New("unknown configuration key")
)

type Crawler struct {
	DBPath     string `toml:"db_path"`
	SeedsPath  string `toml:"seeds_path"`
	ContactURL string `toml:"contact_url"`
}

type API struct {
	DBPath     string `toml:"db_path"`
	ListenAddr string `toml:"listen_addr"`
	CORSOrigin string `toml:"cors_origin"`
}

type TUI struct {
	DBPath string `toml:"db_path"`
}

type file struct {
	Crawler Crawler `toml:"crawler"`
	API     API     `toml:"api"`
	TUI     TUI     `toml:"tui"`
}

// setting is one configuration value: its key in the file, its environment variable and its target.
type setting struct {
	key, env string
	value    *string
	required bool
}

// LoadCrawler reads the [crawler] section of the configuration file. Environment variables override it.
func LoadCrawler() (Crawler, error) {
	f, err := read()
	c := f.Crawler
	if err != nil {
		return c, err
	}
	return c, apply([]setting{
		{key: "crawler.db_path", env: "OAI_DB_PATH", value: &c.DBPath, required: true},
		{key: "crawler.seeds_path", env: "OAI_SEEDS_PATH", value: &c.SeedsPath, required: true},
		{key: "crawler.contact_url", env: "OAI_CONTACT_URL", value: &c.ContactURL, required: true},
	})
}

// LoadAPI reads the [api] section of the configuration file. Environment variables override it.
func LoadAPI() (API, error) {
	f, err := read()
	c := f.API
	if err != nil {
		return c, err
	}
	return c, apply([]setting{
		{key: "api.db_path", env: "OAI_DB_PATH", value: &c.DBPath, required: true},
		{key: "api.listen_addr", env: "OAI_LISTEN_ADDR", value: &c.ListenAddr, required: true},
		{key: "api.cors_origin", env: "OAI_CORS_ORIGIN", value: &c.CORSOrigin},
	})
}

// LoadTUI reads the [tui] section of the configuration file. Environment variables override it.
func LoadTUI() (TUI, error) {
	f, err := read()
	c := f.TUI
	if err != nil {
		return c, err
	}
	return c, apply([]setting{
		{key: "tui.db_path", env: "OAI_DB_PATH", value: &c.DBPath, required: true},
	})
}

// read decodes the configuration file. Without OAI_CONFIG, a missing oai.toml is not an error.
func read() (file, error) {
	path, explicit := os.LookupEnv(EnvPath)
	if !explicit {
		path = DefaultPath
	}
	var f file
	md, err := toml.DecodeFile(path, &f)
	if errors.Is(err, fs.ErrNotExist) && !explicit {
		return file{}, nil
	}
	if err != nil {
		return file{}, fmt.Errorf("config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return file{}, fmt.Errorf("config %s: %w: %s", path, ErrUnknownKey, undecoded[0])
	}
	return f, nil
}

func apply(settings []setting) error {
	var missing []string
	for _, s := range settings {
		if v, ok := os.LookupEnv(s.env); ok {
			*s.value = v
		}
		if s.required && *s.value == "" {
			missing = append(missing, fmt.Sprintf("%s (%s)", s.key, s.env))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissing, strings.Join(missing, ", "))
	}
	return nil
}
