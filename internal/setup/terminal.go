package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// Wordmark is PUNCHCARD in block letters, shown above the wizard.
const Wordmark = "" +
	"█▀█ █ █ █▄ █ █▀▀ █ █ █▀▀ ▄▀█ █▀█ █▀▄\n" +
	"█▀▀ █▄█ █ ▀█ █▄▄ █▀█ █▄▄ █▀█ █▀▄ █▄▀"

var (
	tGutter = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	tTitle  = lipgloss.NewStyle().Bold(true)
	tDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tGood   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	tBad    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	tWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

// Terminal is the UI on a real terminal: clack-style lines in a gutter,
// one Huh form per prompt (Huh is inline: no alt screen). Huh writes to
// stderr unless told otherwise, so every form gets out explicitly.
type Terminal struct {
	in  io.Reader
	tty io.Writer // the terminal itself, for Huh (Bubble Tea sizes it)
	out io.Writer // our own lines, colour-downsampled to its profile
	// width is the forms' width: the terminal's, capped. Huh panics on
	// a terminal that reports no size, so unknown is 72.
	width int
	mu    sync.Mutex
}

// NewTerminal is the wizard's UI on in and out. environ decides the
// colour profile (NO_COLOR, TERM=dumb).
func NewTerminal(in io.Reader, out io.Writer, environ []string) *Terminal {
	width := 72
	if f, ok := out.(*os.File); ok {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w >= 20 {
			width = min(w, 80)
		}
	}
	return &Terminal{in: in, tty: out, out: colorprofile.NewWriter(out, environ), width: width}
}

// Begin prints the wordmark and opens the gutter.
func (t *Terminal) Begin() {
	t.print("\n" + tMark.Render(Wordmark) + "\n\n" + tGutter.Render("┌") + "  " + tTitle.Render("punchcard setup") + "\n")
}

// End closes the gutter.
func (t *Terminal) End() { t.print(tGutter.Render("└") + "\n") }

func (t *Terminal) print(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprint(t.out, s)
}

func (t *Terminal) Say(lines ...string) {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(tGutter.Render("│") + "  " + paint(l) + "\n")
	}
	t.print(b.String())
}

// paint colours a line by its leading mark.
func paint(l string) string {
	switch {
	case strings.HasPrefix(l, "✓"):
		return tGood.Render(l)
	case strings.HasPrefix(l, "✗"):
		return tBad.Render(l)
	case strings.HasPrefix(l, "⚠"):
		return tWarn.Render(l)
	case strings.HasPrefix(l, "$ "):
		return tDim.Render(l)
	}
	return l
}

// answered leaves the step on screen once its form has gone.
func (t *Terminal) answered(title, answer string) {
	t.print(tGutter.Render("◇") + "  " + title + "\n" + tGutter.Render("│") + "  " + tDim.Render(answer) + "\n" + tGutter.Render("│") + "\n")
}

func (t *Terminal) keys() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"))
	return km
}

func (t *Terminal) run(field huh.Field) error {
	err := huh.NewForm(huh.NewGroup(field)).
		WithShowHelp(false).WithWidth(t.width).
		WithKeyMap(t.keys()).
		WithTheme(huh.ThemeFunc(huh.ThemeBase16)).
		WithInput(t.in).
		WithOutput(t.tty).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrAborted
	}
	return err
}

func (t *Terminal) Ask(f Field) (string, error) {
	v := f.Value
	in := huh.NewInput().Title(f.Title).Placeholder(f.Placeholder).Value(&v)
	note := strings.Join(f.Note, "\n")
	if f.Echo != nil {
		in = in.DescriptionFunc(func() string {
			if e := f.Echo(v); e != "" {
				return strings.TrimLeft(note+"\n"+e, "\n")
			}
			return note
		}, &v)
	} else if note != "" {
		in = in.Description(note)
	}
	if f.Secret {
		in = in.EchoMode(huh.EchoModePassword)
	}
	if f.Validate != nil {
		in = in.Validate(f.Validate)
	}
	if err := t.run(in); err != nil {
		return "", err
	}
	shown := strings.TrimSpace(v)
	switch {
	case f.Secret && shown == "":
		shown = "(blank)"
	case f.Secret:
		shown = strings.Repeat("•", min(len(shown), 12))
	case f.Echo != nil:
		if e := f.Echo(v); e != "" {
			shown = strings.TrimPrefix(e, "→ ")
		}
	}
	t.answered(f.Title, shown)
	return v, nil
}

func (t *Terminal) Confirm(q Question) (bool, error) {
	v := q.Default
	c := huh.NewConfirm().Title(q.Title).Affirmative("Yes").Negative("No").Value(&v)
	if len(q.Note) > 0 {
		c = c.Description(strings.Join(q.Note, "\n"))
	}
	if err := t.run(c); err != nil {
		return false, err
	}
	answer := "No"
	if v {
		answer = "Yes"
	}
	t.answered(q.Title, answer)
	return v, nil
}

// Output is a writer for a command's output: each line in the gutter,
// dimmed.
func (t *Terminal) Output() io.Writer { return &gutterWriter{t: t} }

type gutterWriter struct {
	t   *Terminal
	buf bytes.Buffer
}

func (g *gutterWriter) Write(p []byte) (int, error) {
	g.buf.Write(p)
	for {
		i := bytes.IndexByte(g.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(g.buf.Next(i+1)), "\r\n")
		g.t.print(tGutter.Render("│") + "  " + tDim.Render("  "+line) + "\n")
	}
	return len(p), nil
}
