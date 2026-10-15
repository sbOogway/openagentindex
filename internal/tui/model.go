package tui

import (
	"context"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sbOogway/openagentindex/internal/chat"
	"github.com/sbOogway/openagentindex/internal/index"
)

var statuses = []string{index.StatusOK, index.StatusFailed, index.StatusAny}

type Index interface {
	Search(ctx context.Context, q index.Query) (index.Results, error)
	Agent(ctx context.Context, host string) (index.Agent, error)
}

type Chat interface {
	Send(ctx context.Context, card []byte, contextID, text string) (chat.Reply, error)
}

type focus int

const (
	focusSearch focus = iota
	focusChat
)

type message struct {
	from string // "you", "agent" or "error"
	text string
}

// conversation is the chat with one host. It lives while the client runs.
type conversation struct {
	messages  []message
	contextID string
	sending   bool
	scroll    int // lines above the newest line
}

type resultsMsg struct {
	seq int
	res index.Results
	err error
}

type agentMsg struct {
	seq   int
	agent index.Agent
	err   error
}

type replyMsg struct {
	host  string
	reply chat.Reply
	err   error
}

type model struct {
	ctx   context.Context
	index Index
	chat  Chat

	query     textinput.Model
	status    int
	searchSeq int
	results   index.Results
	searchErr error
	cursor    int

	agentSeq int
	agent    *index.Agent
	agentErr error

	focus focus
	input textinput.Model
	convs map[string]*conversation
	note  string

	width, height int
}

func newModel(ctx context.Context, cfg Config) model {
	query := textinput.New()
	query.Prompt = "> "
	query.Placeholder = "name, host or description"
	query.Focus()
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "message"
	return model{
		ctx: ctx, index: cfg.Index, chat: cfg.Chat,
		query: query, input: input, convs: map[string]*conversation{},
		width: 100, height: 30,
	}
}

func (m model) Init() tea.Cmd {
	return m.searchCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case resultsMsg:
		return m.gotResults(msg)
	case agentMsg:
		if msg.seq == m.agentSeq {
			m.agent, m.agentErr = &msg.agent, msg.err
		}
		return m, nil
	case replyMsg:
		m.gotReply(msg)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.focus == focusChat {
			return m.chatKey(msg)
		}
		return m.searchKey(msg)
	}
	return m, nil
}

func (m model) gotResults(msg resultsMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.searchSeq {
		return m, nil
	}
	m.results, m.searchErr, m.cursor = msg.res, msg.err, 0
	return m.selectAgent()
}

func (m *model) gotReply(msg replyMsg) {
	c := m.conv(msg.host)
	c.sending, c.scroll = false, 0
	if msg.err != nil {
		c.messages = append(c.messages, message{from: "error", text: msg.err.Error()})
		return
	}
	c.contextID = msg.reply.ContextID
	text := msg.reply.Text
	if text == "" {
		text = "(empty reply)"
	}
	c.messages = append(c.messages, message{from: "agent", text: text})
}

func (m model) searchKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.note = ""
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "tab":
		m.status = (m.status + 1) % len(statuses)
		return m.newSearch()
	case "up", "ctrl+p":
		return m.moveCursor(-1)
	case "down", "ctrl+n":
		return m.moveCursor(1)
	case "enter":
		return m.openChat()
	}
	before := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() == before {
		return m, cmd
	}
	m, search := m.newSearch()
	return m, tea.Batch(cmd, search)
}

func (m model) chatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" || m.agent == nil {
		return m.closeChat()
	}
	switch msg.String() {
	case "enter":
		return m.send()
	case "pgup":
		m.conv(m.agent.Host).scroll += 5
		return m, nil
	case "pgdown":
		c := m.conv(m.agent.Host)
		c.scroll = max(0, c.scroll-5)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	next := max(0, min(len(m.results.Agents)-1, m.cursor+delta))
	if next == m.cursor {
		return m, nil
	}
	m.cursor = next
	return m.selectAgent()
}

// newSearch starts a search for the current query. Results of older searches are ignored.
func (m model) newSearch() (model, tea.Cmd) {
	m.searchSeq++
	return m, m.searchCmd()
}

func (m model) searchCmd() tea.Cmd {
	ctx, ix, seq := m.ctx, m.index, m.searchSeq
	q := index.Query{Text: m.query.Value(), Status: statuses[m.status], Limit: index.MaxLimit}
	return func() tea.Msg {
		res, err := ix.Search(ctx, q)
		return resultsMsg{seq: seq, res: res, err: err}
	}
}

// selectAgent loads the agent under the cursor. Older loads are ignored.
func (m model) selectAgent() (tea.Model, tea.Cmd) {
	m.agentSeq++
	m.agent, m.agentErr = nil, nil
	if len(m.results.Agents) == 0 {
		return m, nil
	}
	ctx, ix, seq, host := m.ctx, m.index, m.agentSeq, m.results.Agents[m.cursor].Host
	return m, func() tea.Msg {
		a, err := ix.Agent(ctx, host)
		return agentMsg{seq: seq, agent: a, err: err}
	}
}

func (m model) openChat() (tea.Model, tea.Cmd) {
	switch {
	case m.agent == nil:
		return m, nil
	case m.agent.Card == nil:
		m.note = "This host has no valid agent card, so the chat is not available."
		return m, nil
	}
	m.focus = focusChat
	m.query.Blur()
	return m, m.input.Focus()
}

func (m model) closeChat() (tea.Model, tea.Cmd) {
	m.focus = focusSearch
	m.input.Blur()
	return m, m.query.Focus()
}

func (m model) send() (tea.Model, tea.Cmd) {
	text := m.input.Value()
	c := m.conv(m.agent.Host)
	if text == "" || c.sending {
		return m, nil
	}
	m.input.SetValue("")
	c.messages = append(c.messages, message{from: "you", text: text})
	c.sending, c.scroll = true, 0
	ctx, ch, host, card, contextID := m.ctx, m.chat, m.agent.Host, []byte(m.agent.Card), c.contextID
	return m, func() tea.Msg {
		r, err := ch.Send(ctx, card, contextID, text)
		return replyMsg{host: host, reply: r, err: err}
	}
}

func (m model) conv(host string) *conversation {
	c, ok := m.convs[host]
	if !ok {
		c = &conversation{}
		m.convs[host] = c
	}
	return c
}
