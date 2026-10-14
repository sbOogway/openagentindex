package card

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

var ErrInvalid = errors.New("invalid agent card")

var parse = a2av0.NewAgentCardParser()

// Parse decodes a v1.0 or v0.3 agent card and returns it in the v1.0 form.
func Parse(data []byte) (*a2a.AgentCard, error) {
	c, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err := validate(c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return c, nil
}

func validate(c *a2a.AgentCard) error {
	if c.Name == "" {
		return errors.New("no name")
	}
	for _, iface := range c.SupportedInterfaces {
		if iface != nil && iface.ProtocolBinding != "" && isHTTPURL(iface.URL) {
			return nil
		}
	}
	return errors.New("no interface with an http or https URL and a protocol binding")
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
