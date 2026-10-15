package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	topHeight     = 4 // border, title, one line, border
	detailsHeight = 8
	minWidth      = 40
	minChatHeight = 4
	minHeight     = topHeight + detailsHeight + minChatHeight + 1
)

var (
	bold   = lipgloss.NewStyle().Bold(true)
	faint  = lipgloss.NewStyle().Faint(true)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	box    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (m model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// layout holds the panel sizes for the terminal size.
type layout struct {
	leftW, rightW, mainH int
	chatTop, chatH       int
}

func (m model) layout() layout {
	leftW := max(minWidth/2, m.width*2/5)
	mainH := m.height - topHeight - 1
	return layout{
		leftW: leftW, rightW: m.width - leftW, mainH: mainH,
		chatTop: topHeight + detailsHeight, chatH: mainH - detailsHeight,
	}
}

// inChat reports whether the cell x, y is in the chat panel.
func (l layout) inChat(x, y int) bool {
	return x >= l.leftW && y >= l.chatTop && y < l.chatTop+l.chatH
}

func (l layout) chatWidth() int {
	return l.rightW - 4 // borders and padding
}

func (l layout) chatRows() int {
	return max(1, l.chatH-4) // borders, title and input
}

func (m model) screen() string {
	if m.width < minWidth || m.height < minHeight {
		return "The terminal is too small."
	}
	l := m.layout()
	leftW, rightW, mainH := l.leftW, l.rightW, l.mainH
	top := lipgloss.JoinHorizontal(lipgloss.Top,
		panel("Search", []string{inputView(m.query, leftW)}, leftW, topHeight, m.focus == focusSearch),
		panel("Index", []string{m.summary()}, rightW, topHeight, false),
	)
	right := lipgloss.JoinVertical(lipgloss.Left,
		panel(m.detailsTitle(), m.details(rightW-4), rightW, detailsHeight, false),
		m.chatPanel(l),
	)
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		panel("Agents", m.list(mainH-3), leftW, mainH, m.focus == focusSearch),
		right,
	)
	return lipgloss.JoinVertical(lipgloss.Left, top, main, m.help())
}

// panel draws a box of exactly w × h cells with a title line and the body lines.
func panel(title string, body []string, w, h int, focused bool) string {
	inner, rows := max(1, w-4), max(0, h-2)
	lines := append([]string{bold.Render(title)}, body...)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, inner, "…")
	}
	style := box.BorderForeground(lipgloss.Color("8"))
	if focused {
		style = box.BorderForeground(lipgloss.Color("6"))
	}
	return style.Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

func (m model) summary() string {
	status := faint.Render("status: " + statuses[m.status])
	switch {
	case m.searchErr != nil:
		return red.Render(clean(m.searchErr.Error()))
	case m.results.Total > len(m.results.Agents):
		return fmt.Sprintf("first %d of %d agents · %s", len(m.results.Agents), m.results.Total, status)
	case m.results.Total == 1:
		return "1 agent · " + status
	default:
		return fmt.Sprintf("%d agents · %s", m.results.Total, status)
	}
}

// list shows the results that fit in rows lines, around the cursor.
func (m model) list(rows int) []string {
	rows = max(1, rows)
	start := max(0, m.cursor-rows+1)
	end := min(len(m.results.Agents), start+rows)
	var out []string
	for i := start; i < end; i++ {
		a := m.results.Agents[i]
		dot := green.Render("●")
		if !a.OK {
			dot = red.Render("●")
		}
		name := clean(a.Name)
		if name == "" {
			name = faint.Render(clean(a.Host))
		}
		prefix := "  "
		if i == m.cursor {
			prefix = accent.Render("▸ ")
			name = bold.Render(name)
		}
		out = append(out, prefix+dot+" "+name)
	}
	return out
}

func (m model) detailsTitle() string {
	if m.agent == nil {
		return "Agent"
	}
	if name := clean(m.agent.Name); name != "" {
		return name
	}
	return clean(m.agent.Host)
}

func (m model) details(width int) []string {
	switch {
	case m.agentErr != nil:
		return []string{red.Render(clean(m.agentErr.Error()))}
	case m.agent == nil:
		return []string{faint.Render("No agent selected.")}
	}
	a := m.agent
	status := green.Render("ok")
	if !a.OK {
		status = red.Render("failed")
	}
	meta := clean(a.Host) + " · " + status
	if a.ProtocolVersion != "" {
		meta += " · v" + clean(a.ProtocolVersion)
	}
	text := a.Description
	if text == "" {
		text = a.Error
	}
	lines := []string{faint.Render(meta)}
	lines = append(lines, wrap(clean(text), width, 3)...)
	return append(lines, faint.Render("last check: "+lastCheck(a.HTTPStatus, a.LastChecked)))
}

func lastCheck(status int, at time.Time) string {
	return fmt.Sprintf("HTTP %d · %s", status, at.Local().Format("2006-01-02 15:04"))
}

func (m model) chatPanel(l layout) string {
	w, h := l.rightW, l.chatH
	title := "Chat"
	if m.focus == focusSearch {
		title += faint.Render("  enter to start")
	}
	if m.agent == nil || m.agent.Card == nil {
		note := m.note
		if note == "" {
			note = "Select an agent with a valid card."
		}
		return panel(title, []string{faint.Render(note)}, w, h, false)
	}
	c := m.convs[m.agent.Host]
	body := append(chatLog(c, l.chatWidth(), l.chatRows()), inputView(m.input, w))
	return panel(title, body, w, h, m.focus == focusChat)
}

// chatLog returns rows lines of the conversation, from the newest line minus the scroll.
func chatLog(c *conversation, width, rows int) []string {
	lines := chatLines(c, width)
	scroll := 0
	if c != nil {
		scroll = min(c.scroll, max(0, len(lines)-rows))
	}
	end := len(lines) - scroll
	start := max(0, end-rows)
	out := lines[start:end]
	for len(out) < rows {
		out = append([]string{""}, out...)
	}
	return out
}

// chatLines returns all lines of the conversation for the width.
func chatLines(c *conversation, width int) []string {
	var lines []string
	if c != nil {
		for _, msg := range c.messages {
			lines = append(lines, messageLines(msg, width)...)
		}
		if c.sending {
			lines = append(lines, faint.Render("agent › …"))
		}
	}
	if len(lines) == 0 {
		for _, l := range wrap("Messages go from this computer directly to the agent. Some agents charge or need a key.", width, 0) {
			lines = append(lines, faint.Render(l))
		}
	}
	return lines
}

// inputView renders a text input that fills a panel of width w. A text input
// without a width shows only the first letter of its placeholder.
func inputView(in textinput.Model, w int) string {
	in.SetWidth(max(1, w-8))
	return in.View()
}

func messageLines(msg message, width int) []string {
	label := map[string]string{
		"you":   accent.Render("you ›"),
		"agent": green.Render("agent ›"),
		"error": red.Render("error ›"),
	}[msg.from]
	var out []string
	for i, l := range cleanLines(msg.text) {
		prefix := "  "
		if i == 0 {
			prefix = label + " "
		}
		for j, w := range wrap(l, max(10, width-lipgloss.Width(prefix)), 0) {
			if j > 0 {
				prefix = "  "
			}
			out = append(out, prefix+w)
		}
	}
	return out
}

// wrap breaks text into lines of at most width cells. limit > 0 keeps only that many lines.
func wrap(text string, width, limit int) []string {
	lines := strings.Split(lipgloss.NewStyle().Width(max(1, width)).Render(text), "\n")
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = ansi.Truncate(lines[limit-1]+" …", width, "…")
	}
	return lines
}

func (m model) help() string {
	if m.focus == focusChat {
		return faint.Render(" enter send · wheel or pgup/pgdown scroll · esc back · ctrl+c quit")
	}
	return faint.Render(" type to search · ↑↓ select · enter chat · tab status · esc quit")
}
