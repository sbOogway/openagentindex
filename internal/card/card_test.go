package card_test

import (
	"errors"
	"os"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/sbOogway/openagentindex/internal/card"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		file            string
		name            string
		interfaces      int
		protocolVersion a2a.ProtocolVersion
		streaming       bool
		skills          int
	}{
		{file: "testdata/v1.json", name: "Echo Agent", interfaces: 2, protocolVersion: "1.0", streaming: true, skills: 2},
		{file: "testdata/v03.json", name: "Old Agent", interfaces: 2, protocolVersion: "0.3.0", streaming: true, skills: 1},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(tt.file)
			if err != nil {
				t.Fatal(err)
			}
			c, err := card.Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if c.Name != tt.name {
				t.Errorf("name = %q, want %q", c.Name, tt.name)
			}
			if len(c.SupportedInterfaces) != tt.interfaces {
				t.Fatalf("interfaces = %d, want %d", len(c.SupportedInterfaces), tt.interfaces)
			}
			if v := c.SupportedInterfaces[0].ProtocolVersion; v != tt.protocolVersion {
				t.Errorf("protocol version = %q, want %q", v, tt.protocolVersion)
			}
			if c.Capabilities.Streaming != tt.streaming {
				t.Errorf("streaming = %v, want %v", c.Capabilities.Streaming, tt.streaming)
			}
			if len(c.Skills) != tt.skills {
				t.Errorf("skills = %d, want %d", len(c.Skills), tt.skills)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "not JSON", data: `<html></html>`},
		{name: "not a card", data: `{"hello": "world"}`},
		{name: "no name", data: `{"supportedInterfaces": [{"url": "https://a.example", "protocolBinding": "JSONRPC"}]}`},
		{name: "no interfaces", data: `{"name": "A", "supportedInterfaces": []}`},
		{name: "relative URL", data: `{"name": "A", "supportedInterfaces": [{"url": "/a2a", "protocolBinding": "JSONRPC"}]}`},
		{name: "non-HTTP URL", data: `{"name": "A", "supportedInterfaces": [{"url": "ftp://a.example", "protocolBinding": "JSONRPC"}]}`},
		{name: "no binding", data: `{"name": "A", "supportedInterfaces": [{"url": "https://a.example"}]}`},
		{name: "duplicate key", data: `{"name": "A", "name": "B", "supportedInterfaces": [{"url": "https://a.example", "protocolBinding": "JSONRPC"}]}`},
		{name: "nested duplicate key", data: `{"name": "A", "capabilities": {"streaming": true, "streaming": false}, "supportedInterfaces": [{"url": "https://a.example", "protocolBinding": "JSONRPC"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := card.Parse([]byte(tt.data))
			if !errors.Is(err, card.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
