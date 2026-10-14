package leads

import (
	"bufio"
	"context"
	"os"
	"strings"
)

// SeedFile reads hosts from a text file with one host on each line.
// It ignores empty lines and lines that start with "#".
type SeedFile struct {
	Path string
}

func (s SeedFile) Hosts(_ context.Context) ([]string, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hosts []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		host := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if host == "" || strings.HasPrefix(host, "#") || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	return hosts, scanner.Err()
}
