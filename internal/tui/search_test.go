package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sbOogway/openagentindex/internal/index"
)

type fakeIndex struct {
	queries []index.Query
	results map[string]index.Results
	err     error
}

func (f *fakeIndex) Search(_ context.Context, q index.Query) (index.Results, error) {
	f.queries = append(f.queries, q)
	return f.results[q.Text], f.err
}

func agents(names ...string) index.Results {
	r := index.Results{Total: len(names)}
	for _, n := range names {
		r.Agents = append(r.Agents, index.Summary{Host: n + ".example", Name: n, OK: true, ProtocolVersion: "1.0"})
	}
	return r
}

// run executes cmd and returns its messages. It runs the commands of a batch too.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
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

// send gives msg to the model and gives the result messages of its commands back to it.
func send(m tea.Model, msg tea.Msg) (tea.Model, []tea.Msg) {
	m, cmd := m.Update(msg)
	msgs := run(cmd)
	for _, r := range msgs {
		if _, ok := r.(resultsMsg); ok {
			m, _ = m.Update(r)
		}
	}
	return m, msgs
}

func start(t *testing.T, ix *fakeIndex) tea.Model {
	t.Helper()
	var m tea.Model = newSearch(context.Background(), ix)
	for _, msg := range run(m.Init()) {
		m, _ = m.Update(msg)
	}
	return m
}

func typeText(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m, _ = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func content(m tea.Model) string {
	return m.View().Content
}

func TestInitialSearch(t *testing.T) {
	ix := &fakeIndex{results: map[string]index.Results{"": agents("Alpha", "Beta")}}
	m := start(t, ix)
	want := index.Query{Status: index.StatusOK, Limit: index.MaxLimit}
	if len(ix.queries) != 1 || ix.queries[0].Text != want.Text || ix.queries[0].Status != want.Status || ix.queries[0].Limit != want.Limit {
		t.Fatalf("queries = %+v, want one %+v", ix.queries, want)
	}
	for _, s := range []string{"2 agents", "Alpha", "Beta.example"} {
		if !strings.Contains(content(m), s) {
			t.Errorf("view does not contain %q:\n%s", s, content(m))
		}
	}
}

func TestSearchAsYouType(t *testing.T) {
	ix := &fakeIndex{results: map[string]index.Results{"ec": agents("Echo")}}
	m := typeText(start(t, ix), "ec")
	var texts []string
	for _, q := range ix.queries {
		texts = append(texts, q.Text)
	}
	if strings.Join(texts, "|") != "|e|ec" {
		t.Errorf("query texts = %q, want [\"\" e ec]", texts)
	}
	if !strings.Contains(content(m), "Echo") {
		t.Errorf("view does not show the result:\n%s", content(m))
	}
}

func TestOldResultsAreIgnored(t *testing.T) {
	ix := &fakeIndex{results: map[string]index.Results{"": agents("Old"), "n": agents("New")}}
	m := typeText(start(t, ix), "n")
	m, _ = m.Update(resultsMsg{seq: 0, res: agents("Old")})
	if c := content(m); strings.Contains(c, "Old") || !strings.Contains(c, "New") {
		t.Errorf("view after an old result:\n%s", c)
	}
}

func TestTabCyclesStatus(t *testing.T) {
	ix := &fakeIndex{}
	m := start(t, ix)
	for range 3 {
		m, _ = send(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	var got []string
	for _, q := range ix.queries {
		got = append(got, q.Status)
	}
	if strings.Join(got, ",") != "ok,failed,any,ok" {
		t.Errorf("statuses = %v, want ok,failed,any,ok", got)
	}
}

func TestCursor(t *testing.T) {
	ix := &fakeIndex{results: map[string]index.Results{"": agents("A", "B", "C")}}
	m := start(t, ix)
	keys := []rune{tea.KeyUp, tea.KeyDown, tea.KeyDown, tea.KeyDown, tea.KeyDown}
	for _, k := range keys {
		m, _ = send(m, tea.KeyPressMsg{Code: k})
	}
	if got := m.(searchModel).cursor; got != 2 {
		t.Errorf("cursor = %d, want 2 (stays in the list)", got)
	}
	if !strings.Contains(content(m), "▸ ") {
		t.Errorf("view has no marker:\n%s", content(m))
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
		_, msgs := send(start(t, &fakeIndex{}), k)
		if len(msgs) != 1 {
			t.Fatalf("%s: messages = %v, want one quit", k, msgs)
		}
		if _, ok := msgs[0].(tea.QuitMsg); !ok {
			t.Errorf("%s: message = %T, want tea.QuitMsg", k, msgs[0])
		}
	}
}

func TestSearchError(t *testing.T) {
	m := start(t, &fakeIndex{err: errors.New("database is locked")})
	if !strings.Contains(content(m), "database is locked") {
		t.Errorf("view does not show the error:\n%s", content(m))
	}
}

func TestAgentTextIsCleaned(t *testing.T) {
	evil := index.Results{Total: 1, Agents: []index.Summary{{
		Host: "evil.example", Name: "Evil\x1b]0;pwned\x07", Description: "\x1b[2Jline\nbreak",
	}}}
	m := start(t, &fakeIndex{results: map[string]index.Results{"": evil}})
	c := content(m)
	if strings.Contains(c, "\x1b]") || strings.Contains(c, "pwned") || strings.Contains(c, "\x1b[2J") || strings.Contains(c, "line\nbreak") {
		t.Errorf("view contains agent control sequences: %q", c)
	}
	if !strings.Contains(c, "line break") {
		t.Errorf("view does not contain the cleaned description: %q", c)
	}
}

func TestCursorWithoutResults(t *testing.T) {
	m := start(t, &fakeIndex{})
	m, _ = send(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if got := m.(searchModel).cursor; got != 0 {
		t.Errorf("cursor = %d, want 0", got)
	}
	if !strings.Contains(content(m), "0 agents") {
		t.Errorf("view:\n%s", content(m))
	}
}

func TestOneAgent(t *testing.T) {
	m := start(t, &fakeIndex{results: map[string]index.Results{"": agents("Solo")}})
	if !strings.Contains(content(m), "1 agent") || strings.Contains(content(m), "1 agents") {
		t.Errorf("view:\n%s", content(m))
	}
}
