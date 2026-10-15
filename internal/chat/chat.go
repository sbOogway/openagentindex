package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"

	"github.com/sbOogway/openagentindex/internal/card"
)

// Reply is the answer of an agent. ContextID continues the conversation.
type Reply struct {
	Text      string
	ContextID string
}

type Client struct {
	http *http.Client
}

// New returns a client that sends all requests with hc.
func New(hc *http.Client) *Client {
	return &Client{http: hc}
}

// Send sends text to the agent of the raw card and waits for the reply. An
// empty contextID starts a new conversation.
func (c *Client) Send(ctx context.Context, rawCard []byte, contextID, text string) (Reply, error) {
	ac, err := card.Parse(rawCard)
	if err != nil {
		return Reply{}, err
	}
	client, err := a2aclient.NewFromCard(ctx, ac, c.transports()...)
	if err != nil {
		return Reply{}, err
	}
	defer func() { _ = client.Destroy() }()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.ContextID = contextID
	res, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return Reply{}, err
	}
	return reply(res), nil
}

func (c *Client) transports() []a2aclient.FactoryOption {
	return []a2aclient.FactoryOption{
		a2aclient.WithDefaultsDisabled(),
		a2aclient.WithJSONRPCTransport(c.http),
		a2aclient.WithRESTTransport(c.http),
		a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{Client: c.http}),
		a2av0.WithRESTTransport(a2av0.RESTTransportConfig{Client: c.http}),
	}
}

func reply(res a2a.SendMessageResult) Reply {
	switch r := res.(type) {
	case *a2a.Message:
		return Reply{Text: partsText(r.Parts), ContextID: r.ContextID}
	case *a2a.Task:
		lines := []string{"[task " + stateText(r.Status.State) + "]"}
		if r.Status.Message != nil {
			lines = append(lines, partsText(r.Status.Message.Parts))
		}
		for _, a := range r.Artifacts {
			lines = append(lines, partsText(a.Parts))
		}
		return Reply{Text: joinLines(lines), ContextID: r.ContextID}
	}
	return Reply{}
}

func stateText(s a2a.TaskState) string {
	if s == a2a.TaskStateUnspecified {
		return "unknown"
	}
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(string(s), "TASK_STATE_")), "_", " ")
}

// partsText joins text parts without a separator, because agents often send one
// text in chunks. Other parts get their own line.
func partsText(parts a2a.ContentParts) string {
	var b strings.Builder
	for _, p := range parts {
		switch {
		case p.Data() != nil:
			data, _ := json.Marshal(p.Data())
			b.WriteString("\n" + string(data) + "\n")
		case p.Raw() != nil || p.URL() != "":
			b.WriteString("\n[file " + p.Filename + "]\n")
		default:
			b.WriteString(p.Text())
		}
	}
	return strings.TrimSpace(b.String())
}

func joinLines(lines []string) string {
	var out []string
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
