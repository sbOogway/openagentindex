package chat

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/sbOogway/openagentindex/internal/card"
)

// echoAgent replies "echo: <text>" and records the context IDs it gets.
type echoAgent struct {
	mu       sync.Mutex
	contexts []string
}

func (e *echoAgent) Execute(_ context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	e.mu.Lock()
	e.contexts = append(e.contexts, ec.ContextID)
	e.mu.Unlock()
	return func(yield func(a2a.Event, error) bool) {
		msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("echo: "+ec.Message.Parts[0].Text()))
		msg.ContextID = ec.ContextID
		yield(msg, nil)
	}
}

func (*echoAgent) Cancel(context.Context, *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(func(a2a.Event, error) bool) {}
}

func TestSend(t *testing.T) {
	tests := []struct {
		name    string
		handler func(a2asrv.RequestHandler) http.Handler
		card    string
	}{
		{
			name:    "v1.0 JSON-RPC",
			handler: func(h a2asrv.RequestHandler) http.Handler { return a2asrv.NewJSONRPCHandler(h) },
			card:    `{"name":"E","supportedInterfaces":[{"url":%q,"protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`,
		},
		{
			name:    "v1.0 HTTP+JSON",
			handler: func(h a2asrv.RequestHandler) http.Handler { return a2asrv.NewRESTHandler(h) },
			card:    `{"name":"E","supportedInterfaces":[{"url":%q,"protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}]}`,
		},
		{
			name:    "v0.3 JSON-RPC",
			handler: func(h a2asrv.RequestHandler) http.Handler { return a2av0.NewJSONRPCHandler(h) },
			card:    `{"name":"E","protocolVersion":"0.3.0","url":%q,"preferredTransport":"JSONRPC"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &echoAgent{}
			srv := httptest.NewTLSServer(tt.handler(a2asrv.NewHandler(agent)))
			defer srv.Close()
			c := New(srv.Client())
			rawCard := []byte(fmt.Sprintf(tt.card, srv.URL))

			first, err := c.Send(context.Background(), rawCard, "", "hello")
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			if first.Text != "echo: hello" || first.ContextID == "" {
				t.Fatalf("reply = %+v, want echo: hello and a context ID", first)
			}
			second, err := c.Send(context.Background(), rawCard, first.ContextID, "again")
			if err != nil {
				t.Fatalf("second Send: %v", err)
			}
			if second.Text != "echo: again" || second.ContextID != first.ContextID {
				t.Errorf("second reply = %+v, want echo: again in context %s", second, first.ContextID)
			}
			if len(agent.contexts) != 2 || agent.contexts[1] != first.ContextID {
				t.Errorf("agent got contexts %v, want the second to be %s", agent.contexts, first.ContextID)
			}
		})
	}
}

func TestSendErrors(t *testing.T) {
	down := httptest.NewTLSServer(http.NotFoundHandler())
	down.Close()
	tests := []struct {
		name string
		card string
		want error
	}{
		{name: "invalid card", card: `{"name":""}`, want: card.ErrInvalid},
		{name: "agent does not respond", card: fmt.Sprintf(`{"name":"E","supportedInterfaces":[{"url":%q,"protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`, down.URL)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(down.Client()).Send(context.Background(), []byte(tt.card), "", "hi")
			if err == nil {
				t.Fatal("Send: no error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReply(t *testing.T) {
	file := a2a.NewRawPart([]byte("x"))
	file.Filename = "report.pdf"
	tests := []struct {
		name string
		res  a2a.SendMessageResult
		want Reply
	}{
		{
			name: "message with text chunks and data",
			res: &a2a.Message{ContextID: "c1", Parts: a2a.ContentParts{
				a2a.NewTextPart("the quick"), a2a.NewTextPart(" brown fox"), a2a.NewDataPart(map[string]any{"n": 1}),
			}},
			want: Reply{Text: "the quick brown fox\n{\"n\":1}", ContextID: "c1"},
		},
		{
			name: "completed task with status message and artifacts",
			res: &a2a.Task{ContextID: "c2", Status: a2a.TaskStatus{
				State:   a2a.TaskStateCompleted,
				Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("Done.")),
			}, Artifacts: []*a2a.Artifact{{Parts: a2a.ContentParts{a2a.NewTextPart("result"), file}}}},
			want: Reply{Text: "[task completed]\nDone.\nresult\n[file report.pdf]", ContextID: "c2"},
		},
		{
			name: "task that needs authentication",
			res:  &a2a.Task{ContextID: "c3", Status: a2a.TaskStatus{State: a2a.TaskStateAuthRequired}},
			want: Reply{Text: "[task auth required]", ContextID: "c3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reply(tt.res); got != tt.want {
				t.Errorf("reply = %+v, want %+v", got, tt.want)
			}
		})
	}
}
