package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sbOogway/openagentindex/internal/chat"
	"github.com/sbOogway/openagentindex/internal/index"
)

type fakeIndex struct {
	queries []index.Query
	results map[string]index.Results
	agents  map[string]index.Agent
	err     error
}

func (f *fakeIndex) Search(_ context.Context, q index.Query) (index.Results, error) {
	f.queries = append(f.queries, q)
	return f.results[q.Text], f.err
}

func (f *fakeIndex) Agent(_ context.Context, host string) (index.Agent, error) {
	a, ok := f.agents[host]
	if !ok {
		return index.Agent{}, fmt.Errorf("%w: %s", index.ErrNotFound, host)
	}
	return a, nil
}

type sent struct {
	card, contextID, text string
}

type fakeChat struct {
	sent  []sent
	reply func(text string) (chat.Reply, error)
}

func (f *fakeChat) Send(_ context.Context, card []byte, contextID, text string) (chat.Reply, error) {
	f.sent = append(f.sent, sent{card: string(card), contextID: contextID, text: text})
	if f.reply != nil {
		return f.reply(text)
	}
	return chat.Reply{Text: "echo: " + text, ContextID: "ctx-1"}, nil
}

// fixture is an index with two agents with cards and one host without a card.
func fixture() (*fakeIndex, *fakeChat) {
	ix := &fakeIndex{results: map[string]index.Results{}, agents: map[string]index.Agent{}}
	var all index.Results
	for _, a := range []struct{ host, name, desc string }{
		{"alpha.example", "Alpha", "First agent"},
		{"beta.example", "Beta", "Second agent"},
		{"dead.example", "", ""},
	} {
		s := index.Summary{Host: a.host, Name: a.name, Description: a.desc, OK: a.name != "", HTTPStatus: 200, ProtocolVersion: "1.0"}
		agent := index.Agent{Summary: s}
		if a.name != "" {
			agent.Card = json.RawMessage(`{"name":"` + a.name + `"}`)
		}
		ix.agents[a.host] = agent
		all.Agents = append(all.Agents, s)
	}
	all.Total = len(all.Agents)
	ix.results[""] = all
	ix.results["al"] = index.Results{Total: 1, Agents: all.Agents[:1]}
	return ix, &fakeChat{}
}

// run executes cmd and returns its messages. It runs the commands of a batch
// too. It drops commands that wait, such as the cursor blink timer.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(50 * time.Millisecond):
		return nil
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, run(c)...)
	}
	return out
}

// send gives msg to the model and gives back all messages of the commands that
// follow, except quit. It returns the quit message, if any.
func send(m tea.Model, msg tea.Msg) (tea.Model, tea.Msg) {
	queue := []tea.Msg{msg}
	var quit tea.Msg
	for len(queue) > 0 {
		var cmd tea.Cmd
		m, cmd = m.Update(queue[0])
		queue = queue[1:]
		for _, r := range run(cmd) {
			switch r.(type) {
			case tea.QuitMsg:
				quit = r
			case resultsMsg, agentMsg, replyMsg:
				queue = append(queue, r)
			}
		}
	}
	return m, quit
}

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func start(t *testing.T, ix *fakeIndex, ch *fakeChat) tea.Model {
	t.Helper()
	var m tea.Model = newModel(context.Background(), Config{Index: ix, Chat: ch})
	m, _ = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, msg := range run(m.Init()) {
		m, _ = send(m, msg)
	}
	return m
}

func typeText(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m, _ = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// screen returns the text on the screen without colours and styles.
func screen(m tea.Model) string {
	return ansi.Strip(m.View().Content)
}

func assertScreen(t *testing.T, m tea.Model, want ...string) {
	t.Helper()
	s := screen(m)
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("screen does not contain %q:\n%s", w, s)
		}
	}
}

func TestStartShowsResultsAndFirstAgent(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	if q := ix.queries[0]; q.Text != "" || q.Status != index.StatusOK || q.Limit != index.MaxLimit {
		t.Errorf("first query = %+v", q)
	}
	assertScreen(t, m, "3 agents", "Alpha", "Beta", "dead.example", "alpha.example · ok · v1.0", "First agent")
}

func TestSearchAsYouType(t *testing.T) {
	ix, ch := fixture()
	m := typeText(start(t, ix, ch), "al")
	var texts []string
	for _, q := range ix.queries {
		texts = append(texts, q.Text)
	}
	if strings.Join(texts, "|") != "|a|al" {
		t.Errorf("query texts = %q", texts)
	}
	assertScreen(t, m, "1 agent ·")
}

func TestOldResultsAreIgnored(t *testing.T) {
	ix, ch := fixture()
	m := typeText(start(t, ix, ch), "al")
	m, _ = send(m, resultsMsg{seq: 0, res: ix.results[""]})
	if strings.Contains(screen(m), "Beta") {
		t.Errorf("an old result replaced the new one:\n%s", screen(m))
	}
}

func TestTabCyclesStatus(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	for range 3 {
		m, _ = send(m, key(tea.KeyTab))
	}
	var got []string
	for _, q := range ix.queries {
		got = append(got, q.Status)
	}
	if strings.Join(got, ",") != "ok,failed,any,ok" {
		t.Errorf("statuses = %v", got)
	}
}

func TestCursorSelectsAgent(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	for _, k := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyDown, tea.KeyDown} {
		m, _ = send(m, key(k))
	}
	if got := m.(model).cursor; got != 2 {
		t.Errorf("cursor = %d, want 2", got)
	}
	assertScreen(t, m, "dead.example · failed")
}

func TestChat(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyEnter))
	if m.(model).focus != focusChat {
		t.Fatal("enter did not open the chat")
	}
	m = typeText(m, "hi")
	m, _ = send(m, key(tea.KeyEnter))
	m = typeText(m, "again")
	m, _ = send(m, key(tea.KeyEnter))

	want := []sent{
		{card: `{"name":"Alpha"}`, contextID: "", text: "hi"},
		{card: `{"name":"Alpha"}`, contextID: "ctx-1", text: "again"},
	}
	if fmt.Sprint(ch.sent) != fmt.Sprint(want) {
		t.Errorf("sent = %+v, want %+v", ch.sent, want)
	}
	assertScreen(t, m, "you › hi", "agent › echo: hi", "agent › echo: again")
}

func TestConversationStaysWithItsAgent(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyEnter))
	m = typeText(m, "hi alpha")
	m, _ = send(m, key(tea.KeyEnter))
	m, _ = send(m, key(tea.KeyEscape))
	if m.(model).focus != focusSearch {
		t.Fatal("esc did not close the chat")
	}
	m, _ = send(m, key(tea.KeyDown))
	if strings.Contains(screen(m), "hi alpha") {
		t.Errorf("the chat of Beta shows the messages of Alpha:\n%s", screen(m))
	}
	m, _ = send(m, key(tea.KeyUp))
	assertScreen(t, m, "you › hi alpha")
}

func TestChatNotAvailableWithoutCard(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyDown))
	m, _ = send(m, key(tea.KeyDown))
	m, _ = send(m, key(tea.KeyEnter))
	if m.(model).focus != focusSearch {
		t.Error("enter opened the chat of a host without a card")
	}
	assertScreen(t, m, "no valid agent card")
}

func TestChatError(t *testing.T) {
	ix, ch := fixture()
	ch.reply = func(string) (chat.Reply, error) { return chat.Reply{}, errors.New("connection refused") }
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyEnter))
	m = typeText(m, "hi")
	m, _ = send(m, key(tea.KeyEnter))
	assertScreen(t, m, "error › connection refused")
}

func TestReplyIsCleaned(t *testing.T) {
	ix, ch := fixture()
	ch.reply = func(string) (chat.Reply, error) {
		return chat.Reply{Text: "line one\x1b]0;pwned\x07\nline two\x1b[2J"}, nil
	}
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyEnter))
	m = typeText(m, "hi")
	m, _ = send(m, key(tea.KeyEnter))
	s := m.View().Content
	if strings.Contains(s, "pwned") || strings.Contains(s, "\x1b]") || strings.Contains(s, "\x1b[2J") {
		t.Errorf("screen contains agent control sequences: %q", s)
	}
	assertScreen(t, m, "agent › line one", "  line two")
}

func TestQuit(t *testing.T) {
	ix, ch := fixture()
	for _, k := range []tea.KeyPressMsg{key(tea.KeyEscape), {Code: 'c', Mod: tea.ModCtrl}} {
		if _, quit := send(start(t, ix, ch), k); quit == nil {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestSmallTerminal(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	sizes := []tea.WindowSizeMsg{{Width: 10, Height: 5}, {Width: 40, Height: 14}, {Width: minWidth, Height: minHeight}, {Width: 300, Height: 100}}
	for _, size := range sizes {
		m, _ = send(m, size)
		_ = screen(m)
	}
	m, _ = send(m, tea.WindowSizeMsg{Width: 10, Height: 5})
	assertScreen(t, m, "too small")
}

func TestSearchError(t *testing.T) {
	ix, ch := fixture()
	ix.err = errors.New("database is locked")
	assertScreen(t, start(t, ix, ch), "database is locked")
}

// longChat returns a model with the chat of Alpha open and 12 messages sent.
func longChat(t *testing.T) tea.Model {
	t.Helper()
	ix, ch := fixture()
	m := start(t, ix, ch)
	m, _ = send(m, key(tea.KeyEnter))
	for i := range 12 {
		m, _ = send(m, tea.PasteMsg{Content: fmt.Sprintf("msg %02d", i)})
		m, _ = send(m, key(tea.KeyEnter))
	}
	return m
}

// wheel turns the mouse wheel at column x of row 20, which is in the chat panel
// for x of 40 or more, and in the agent list for smaller x.
func wheel(m tea.Model, x int, button tea.MouseButton, times int) tea.Model {
	for range times {
		m, _ = send(m, tea.MouseWheelMsg{X: x, Y: 20, Button: button})
	}
	return m
}

func chatScroll(m tea.Model) int {
	return m.(model).convs["alpha.example"].scroll
}

func TestWheelScrollsChat(t *testing.T) {
	m := longChat(t)
	if strings.Contains(screen(m), "you › msg 00") {
		t.Fatalf("the oldest message is visible before scrolling:\n%s", screen(m))
	}
	m = wheel(m, 70, tea.MouseWheelUp, 10)
	assertScreen(t, m, "you › msg 00")
	m = wheel(m, 70, tea.MouseWheelDown, 10)
	assertScreen(t, m, "agent › echo: msg 11")
	if got := chatScroll(m); got != 0 {
		t.Errorf("scroll after scrolling back down = %d, want 0", got)
	}
}

func TestWheelOutsideChat(t *testing.T) {
	m := longChat(t)
	m = wheel(m, 5, tea.MouseWheelUp, 5)
	if got := chatScroll(m); got != 0 {
		t.Errorf("wheel over the agent list scrolled the chat: scroll = %d", got)
	}
}

func TestScrollStopsAtOldestLine(t *testing.T) {
	m := longChat(t)
	m = wheel(m, 70, tea.MouseWheelUp, 100)
	top := chatScroll(m)
	m, _ = send(m, key(tea.KeyPgUp))
	if got := chatScroll(m); got != top {
		t.Errorf("pgup at the top moved the scroll from %d to %d", top, got)
	}
	m = wheel(m, 70, tea.MouseWheelDown, 1)
	if got := chatScroll(m); got != top-wheelLines {
		t.Errorf("one wheel step down from the top: scroll = %d, want %d", got, top-wheelLines)
	}
}

func TestMouseMode(t *testing.T) {
	ix, ch := fixture()
	if mode := start(t, ix, ch).View().MouseMode; mode != tea.MouseModeCellMotion {
		t.Errorf("mouse mode = %v, want cell motion", mode)
	}
}

func TestPaste(t *testing.T) {
	ix, ch := fixture()
	m := start(t, ix, ch)
	m, _ = send(m, tea.PasteMsg{Content: "al"})
	if q := ix.queries[len(ix.queries)-1]; q.Text != "al" {
		t.Errorf("paste in the search box: last query = %q, want al", q.Text)
	}
	m, _ = send(m, key(tea.KeyEnter))
	m, _ = send(m, tea.PasteMsg{Content: "pasted message"})
	_, _ = send(m, key(tea.KeyEnter))
	if len(ch.sent) != 1 || ch.sent[0].text != "pasted message" {
		t.Errorf("paste in the chat: sent = %+v", ch.sent)
	}
}
