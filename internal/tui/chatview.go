package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// bubbleShare is the part of the log width, in percent, that one message can use.
	bubbleShare   = 85
	maxInputLines = 4
	maxInputChars = 8000
	hint          = "Messages go from this computer directly to the agent. Some agents charge or need a key."
)

// chatView is the chat panel: the log of one conversation and the input.
type chatView struct {
	log   viewport.Model
	input textarea.Model
	dark  bool

	renderer      *glamour.TermRenderer
	rendererWidth int
	rendererDark  bool
	// key of the shown content, so that Markdown is rendered only after a change
	shown string
}

func newChatView() chatView {
	in := textarea.New()
	in.Prompt = "> "
	in.Placeholder = "message (alt+enter: new line)"
	in.ShowLineNumbers = false
	in.DynamicHeight = true
	in.MinHeight = 1
	in.MaxHeight = maxInputLines
	in.CharLimit = maxInputChars
	in.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter", "ctrl+j"))
	log := viewport.New()
	log.MouseWheelDelta = wheelLines
	return chatView{log: log, input: in, dark: true}
}

// setSize gives the panel content area width × rows to the log and the input.
func (v *chatView) setSize(width, rows int) {
	v.input.SetWidth(width)
	v.log.SetWidth(width)
	v.log.SetHeight(max(1, rows-v.input.Height()))
}

// show renders the conversation c into the log if it changed. If the log was at
// the newest line, it stays there.
func (v *chatView) show(host string, c *conversation) {
	k := fmt.Sprintf("%s|%d|%t|%d|%t", host, len(c.messages), c.sending, v.log.Width(), v.dark)
	if k == v.shown {
		return
	}
	newHost := !strings.HasPrefix(v.shown, host+"|")
	atBottom := v.log.AtBottom()
	v.shown = k
	v.log.SetContent(v.content(c))
	if newHost || atBottom {
		v.log.GotoBottom()
	}
}

func (v *chatView) content(c *conversation) string {
	width := v.log.Width()
	var parts []string
	for _, msg := range c.messages {
		parts = append(parts, v.message(msg, width))
	}
	if c.sending {
		parts = append(parts, faint.Render("agent › …"))
	}
	if len(parts) == 0 {
		parts = append(parts, faint.Render(strings.Join(wrap(hint, width, 0), "\n")))
	}
	return strings.Join(parts, "\n")
}

var (
	youColor   = lipgloss.Color("6")
	agentColor = lipgloss.Color("2")
	errorColor = lipgloss.Color("1")
)

// message draws msg as a box: the user on the right, the agent and errors on the left.
func (v *chatView) message(msg message, width int) string {
	inner := max(10, width*bubbleShare/100-4) // borders and padding
	switch msg.from {
	case "you":
		box := bubble("you", wrapLines(cleanLines(msg.text), inner), youColor)
		return lipgloss.PlaceHorizontal(width, lipgloss.Right, box)
	case "error":
		return bubble("error", wrapLines(cleanLines(msg.text), inner), errorColor)
	}
	return bubble("agent", strings.Split(v.markdown(msg.text, inner), "\n"), agentColor)
}

func bubble(label string, lines []string, c color.Color) string {
	body := append([]string{lipgloss.NewStyle().Foreground(c).Bold(true).Render(label)}, lines...)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c).
		Padding(0, 1).
		Render(strings.Join(body, "\n"))
}

// markdown renders untrusted agent text as Markdown, or as plain text if that fails.
func (v *chatView) markdown(text string, width int) string {
	r, err := v.markdownRenderer(width)
	if err == nil {
		out, err := r.Render(cleanMarkdown(text))
		if err == nil {
			lines := strings.Split(strings.Trim(stripHyperlinks(out), "\n"), "\n")
			for i, l := range lines {
				lines[i] = trimPadding(l)
			}
			return strings.Join(lines, "\n")
		}
	}
	return strings.Join(wrapLines(cleanLines(text), width), "\n")
}

func (v *chatView) markdownRenderer(width int) (*glamour.TermRenderer, error) {
	if v.renderer != nil && v.rendererWidth == width && v.rendererDark == v.dark {
		return v.renderer, nil
	}
	style := styles.DarkStyleConfig
	if !v.dark {
		style = styles.LightStyleConfig
	}
	noMargin := uint(0)
	style.Document.Margin = &noMargin // the message box has its own padding
	// Many agents send plain text, where each line break counts.
	r, err := glamour.NewTermRenderer(glamour.WithStyles(style), glamour.WithWordWrap(width), glamour.WithPreservedNewLines())
	if err != nil {
		return nil, err
	}
	v.renderer, v.rendererWidth, v.rendererDark = r, width, v.dark
	return r, nil
}

// trimPadding removes the spaces after the text of a line, which glamour adds up
// to the wrap width. It keeps the styles of the text.
func trimPadding(line string) string {
	text := strings.TrimRight(ansi.Strip(line), " ")
	return ansi.Truncate(line, ansi.StringWidth(text), "")
}

// wrapLines wraps each line to width, without padding the short lines.
func wrapLines(lines []string, width int) []string {
	var out []string
	for _, l := range lines {
		for _, w := range wrap(l, width, 0) {
			out = append(out, trimPadding(w))
		}
	}
	return out
}

// scroll gives a mouse wheel or page key message to the log.
func (v *chatView) scroll(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.log, cmd = v.log.Update(msg)
	return cmd
}

// typed gives a message to the input.
func (v *chatView) typed(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return cmd
}

// take returns the text of the input and clears it.
func (v *chatView) take() string {
	text := strings.TrimSpace(v.input.Value())
	v.input.Reset()
	return text
}

func (v chatView) view() []string {
	return append(strings.Split(v.log.View(), "\n"), strings.Split(v.input.View(), "\n")...)
}
