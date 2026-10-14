package leads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	a2aRegistryURL = "https://a2aregistry.org/api/agents"
	// The API returns at most 100 agents on a page. A page of 100 agents is
	// near the 1 MiB body limit of the fetcher.
	a2aRegistryPageSize = 50
)

type Getter interface {
	Get(ctx context.Context, url string) (int, []byte, error)
}

// A2ARegistry reads the hosts of the agents listed on a2aregistry.org.
type A2ARegistry struct {
	get Getter
}

func NewA2ARegistry(g Getter) *A2ARegistry {
	return &A2ARegistry{get: g}
}

type a2aRegistryPage struct {
	Agents []struct {
		WellKnownURI string `json:"wellKnownURI"`
	} `json:"agents"`
	Total int `json:"total"`
}

func (r *A2ARegistry) Hosts(ctx context.Context) ([]string, error) {
	var hosts []string
	for offset := 0; ; {
		page, err := r.page(ctx, offset)
		if err != nil {
			return nil, err
		}
		for _, a := range page.Agents {
			if host, ok := httpsHost(a.WellKnownURI); ok {
				hosts = append(hosts, host)
			}
		}
		offset += len(page.Agents)
		if len(page.Agents) == 0 || offset >= page.Total {
			return hosts, nil
		}
	}
}

func (r *A2ARegistry) page(ctx context.Context, offset int) (a2aRegistryPage, error) {
	q := url.Values{"limit": {strconv.Itoa(a2aRegistryPageSize)}, "offset": {strconv.Itoa(offset)}}
	status, body, err := r.get.Get(ctx, a2aRegistryURL+"?"+q.Encode())
	if err != nil {
		return a2aRegistryPage{}, fmt.Errorf("a2aregistry.org: %w", err)
	}
	if status != http.StatusOK {
		return a2aRegistryPage{}, fmt.Errorf("a2aregistry.org: status %d", status)
	}
	var page a2aRegistryPage
	if err := json.Unmarshal(body, &page); err != nil {
		return a2aRegistryPage{}, fmt.Errorf("a2aregistry.org: %w", err)
	}
	return page, nil
}

func httpsHost(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", false
	}
	return strings.ToLower(u.Host), true
}
