package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sbOogway/openagentindex/internal/index"
)

// headerLines is the number of lines above the result list.
const headerLines = 4

var (
	statuses = []string{index.StatusOK, index.StatusFailed, index.StatusAny}

	bold   = lipgloss.NewStyle().Bold(true)
	faint  = lipgloss.NewStyle().Faint(true)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	marker = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

type Index interface {
	Search(ctx context.Context, q index.Query) (index.Results, error)
}

// resultsMsg carries the results of the search with the number seq.
type resultsMsg struct {
	seq int
	res index.Results
	err error
}

type searchModel struct {
	ctx     context.Context
	index   Index
	input   textinput.Model
	status  int
	seq     int
	results index.Results
	err     error
	cursor  int
	width   int
	height  int
}

func newSearch(ctx context.Context, ix Index) searchModel {
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "Search name, host or description"
	in.Focus()
	return searchModel{ctx: ctx, index: ix, input: in, width: 80, height: 24}
}

func (m searchModel) Init() tea.Cmd {
	return m.searchCmd()
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(msg.Width - 4)
		return m, nil
	case resultsMsg:
		if msg.seq == m.seq {
			m.results, m.err = msg.res, msg.err
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m searchModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "tab":
		m.status = (m.status + 1) % len(statuses)
		return m.newSearch()
	case "up", "ctrl+p":
		m.cursor = max(0, m.cursor-1)
		return m, nil
	case "down", "ctrl+n":
		m.cursor = max(0, min(len(m.results.Agents)-1, m.cursor+1))
		return m, nil
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() == before {
		return m, cmd
	}
	m, search := m.newSearch()
	return m, tea.Batch(cmd, search)
}

// newSearch starts a search for the current query. Results of older searches are ignored.
func (m searchModel) newSearch() (searchModel, tea.Cmd) {
	m.seq++
	m.cursor = 0
	return m, m.searchCmd()
}

func (m searchModel) searchCmd() tea.Cmd {
	ctx, ix, seq := m.ctx, m.index, m.seq
	q := index.Query{Text: m.input.Value(), Status: statuses[m.status], Limit: index.MaxLimit}
	return func() tea.Msg {
		res, err := ix.Search(ctx, q)
		return resultsMsg{seq: seq, res: res, err: err}
	}
}

func (m searchModel) View() tea.View {
	lines := []string{
		bold.Render("Open Agent Index") + "  " + m.summary(),
		m.input.View(),
		faint.Render(fmt.Sprintf("status: %s (tab)  ↑↓ select  esc quit", statuses[m.status])),
		"",
	}
	lines = append(lines, m.rows()...)
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func (m searchModel) summary() string {
	switch {
	case m.err != nil:
		return red.Render(clean(m.err.Error()))
	case m.results.Total > len(m.results.Agents):
		return faint.Render(fmt.Sprintf("first %d of %d", len(m.results.Agents), m.results.Total))
	case m.results.Total == 1:
		return faint.Render("1 agent")
	default:
		return faint.Render(fmt.Sprintf("%d agents", m.results.Total))
	}
}

// rows renders two lines for each result that fits on the screen, around the cursor.
func (m searchModel) rows() []string {
	visible := max(1, (m.height-headerLines)/2)
	start := max(0, m.cursor-visible+1)
	end := min(len(m.results.Agents), start+visible)
	var out []string
	for i := start; i < end; i++ {
		out = append(out, m.row(i)...)
	}
	return out
}

func (m searchModel) row(i int) []string {
	a := m.results.Agents[i]
	prefix := "  "
	if i == m.cursor {
		prefix = marker.Render("▸ ")
	}
	name := clean(a.Name)
	if name == "" {
		name = clean(a.Host)
	}
	status := green.Render("ok")
	if !a.OK {
		status = red.Render("failed")
	}
	title := fmt.Sprintf("%s%s  %s  %s", prefix, bold.Render(name), faint.Render(clean(a.Host)), status)
	if a.ProtocolVersion != "" {
		title += faint.Render("  v" + clean(a.ProtocolVersion))
	}
	detail := a.Description
	if detail == "" {
		detail = a.Error
	}
	return []string{
		ansi.Truncate(title, m.width, "…"),
		faint.Render(ansi.Truncate("  "+clean(detail), m.width, "…")),
	}
}
