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
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/j-dsmith/punchcard/internal/tui"
)

// Wordmark is PUNCHCARD as holes in a punched card: a 4×7 dot matrix
// per letter, one column between letters. A punched position is '#',
// an unpunched one '.'; wordmark renders them as a square hole and a
// faint dot, so the unpunched grid reads as the card itself.
const Wordmark = "" +
	"###..#..#.#..#..##..#..#..##...##..###..###.\n" +
	"#..#.#..#.##.#.#..#.#..#.#..#.#..#.#..#.#..#\n" +
	"#..#.#..#.##.#.#....#..#.#....#..#.#..#.#..#\n" +
	"###..#..#.#.##.#....####.#....####.###..#..#\n" +
	"#....#..#.#.##.#....#..#.#....#..#.#.#..#..#\n" +
	"#....#..#.#..#.#..#.#..#.#..#.#..#.#..#.#..#\n" +
	"#.....##..#..#..##..#..#..##..#..#.#..#.###."

// wordmark is Wordmark punched into a card: square holes shaded down
// the rows from the brand colour, unpunched ones faint, inside a
// hairline edge whose top-left corner is cut, as an IBM card's is.
// Square holes and the edge are from the IBM PC character set, which
// every terminal font has.
func (t *Terminal) wordmark() string {
	rows := strings.Split(Wordmark, "\n")
	inner := len(rows[0]) + 2
	var b strings.Builder
	b.WriteString(" " + tGutter.Render("╱"+strings.Repeat("─", inner-1)+"┐") + "\n")
	for i, row := range rows {
		left := "│"
		if i == 0 {
			left = "╱"
		}
		b.WriteString(tGutter.Render(left) + " ")
		for _, r := range row {
			if r == '#' {
				b.WriteString(t.holes[i].Render("■"))
			} else {
				b.WriteString(t.grid.Render("·"))
			}
		}
		b.WriteString(" " + tGutter.Render("│") + "\n")
	}
	b.WriteString(tGutter.Render("└" + strings.Repeat("─", inner) + "┘"))
	return b.String()
}

// holeShades step the wordmark's seven rows down from the brand colour,
// lit from above: chartreuse to olive on a dark background, and from
// the light-background shade darker still on a light one.
var holeShades = map[bool][]string{
	true:  {"#C8FF00", "#B3E600", "#9ECC00", "#89B200", "#749800", "#5F7E00", "#4A6400"},
	false: {"#5F8700", "#577C00", "#4F7000", "#476500", "#3F5900", "#374E00", "#2E4200"},
}

// holes is the wordmark's hole style per row, for a dark or a light
// background.
func holes(dark bool) []lipgloss.Style {
	var out []lipgloss.Style
	for _, c := range holeShades[dark] {
		out = append(out, lipgloss.NewStyle().Foreground(lipgloss.Color(c)))
	}
	return out
}

// grid is the wordmark's unpunched holes: barely there, so the punched
// ones carry the letters.
func grid(dark bool) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.LightDark(dark)(lipgloss.Color("250"), lipgloss.Color("238")))
}

var (
	tGutter = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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
	// holes and grid are the wordmark's styles, for a dark background
	// until the terminal says its background is light.
	holes []lipgloss.Style
	grid  lipgloss.Style
	// dark is whether the terminal's background is dark, as asked once in
	// NewTerminal; the forms take the brand shade from it. Unknown is dark.
	dark bool
	mu   sync.Mutex
}

// NewTerminal is the wizard's UI on in and out. environ decides the
// colour profile (NO_COLOR, TERM=dumb).
func NewTerminal(in io.Reader, out io.Writer, environ []string) *Terminal {
	width := 72
	t := &Terminal{in: in, tty: out, out: colorprofile.NewWriter(out, environ), width: width, dark: true, holes: holes(true), grid: grid(true)}
	if f, ok := out.(*os.File); ok {
		if w, _, err := term.GetSize(f.Fd()); err == nil && w >= 20 {
			t.width = min(w, 80)
		}
		if fin, ok := in.(*os.File); ok {
			t.dark = lipgloss.HasDarkBackground(fin, f)
			t.holes, t.grid = holes(t.dark), grid(t.dark)
		}
	}
	return t
}

// theme is Huh's base16 theme with the brand colour where that theme
// uses cyan, yellow and magenta: titles, the prompt and cursor, the
// select indicators and the focused button. Descriptions stay grey and
// errors red, so the wizard reads as one thing with the wordmark. The
// shade comes from NewTerminal's one query, not Huh's own, which can
// miss the terminal's answer and fall back to light.
func (term *Terminal) theme(bool) *huh.Styles {
	isDark := term.dark
	t := huh.ThemeBase16(isDark)
	c := lipgloss.LightDark(isDark)(tui.BrandLight, tui.BrandDark)
	t.Focused.Title = t.Focused.Title.Foreground(c).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(c).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(c)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(c)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(c)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(c)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(c)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(lipgloss.Color("0")).Background(c).Bold(true)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(c)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(c)
	t.Group.Title = t.Focused.Title
	return t
}

// Begin prints the wordmark and opens the gutter.
func (t *Terminal) Begin() {
	t.print("\n" + t.wordmark() + "\n\n" + tGutter.Render("┌") + "  " + tTitle.Render("punchcard setup") + "\n")
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

// answered leaves the step on screen once its form has gone. The
// answer is in the terminal's own foreground: ANSI 8 is near the
// background on many dark themes.
func (t *Terminal) answered(title, answer string) {
	t.print(tGutter.Render("◇") + "  " + title + "\n" + tGutter.Render("│") + "  " + answer + "\n" + tGutter.Render("│") + "\n")
}

func (t *Terminal) keys() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"))
	return km
}

func (t *Terminal) run(field huh.Field) error {
	// height is the last frame Huh drew. On submit Huh only clears from
	// the cursor, on that frame's last line, down; the rest of the frame
	// is erased here. Should a later Huh clear it all, this is a no-op.
	// Huh calls the hook before it fills the view in, so the hook asks
	// the form for the frame itself.
	height := 0
	f := huh.NewForm(huh.NewGroup(field))
	err := f.WithShowHelp(false).WithWidth(t.width).
		WithKeyMap(t.keys()).
		WithTheme(huh.ThemeFunc(t.theme)).
		WithInput(t.in).
		WithOutput(t.tty).
		WithViewHook(func(v tea.View) tea.View {
			if frame := f.View(); frame != "" {
				height = lipgloss.Height(frame)
			}
			return v
		}).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrAborted
	}
	if err == nil && height > 1 {
		t.mu.Lock()
		fmt.Fprint(t.tty, ansi.CursorUp(height-1)+"\r"+ansi.EraseScreenBelow)
		t.mu.Unlock()
	}
	return err
}

// typed is an input's value. Huh sets it on its update loop but runs
// a DescriptionFunc on a goroutine of its own, so the two meet under
// mu. The binding Huh hashes is &value alone (hashing reads the mutex
// too, unguarded), on the update loop, where value is also set.
type typed struct {
	mu    sync.Mutex
	value string
}

func (a *typed) Get() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.value
}

func (a *typed) Set(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.value = v
}

func (t *Terminal) Ask(f Field) (string, error) {
	acc := &typed{value: f.Value}
	in := huh.NewInput().Title(f.Title).Placeholder(f.Placeholder).Accessor(acc)
	note := strings.Join(f.Note, "\n")
	if f.Echo != nil {
		in = in.DescriptionFunc(func() string {
			if e := f.Echo(acc.Get()); e != "" {
				return strings.TrimLeft(note+"\n"+e, "\n")
			}
			return note
		}, &acc.value)
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
	v := acc.Get()
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

func (t *Terminal) Choose(c Choice) (string, error) {
	v := c.Value
	opts := make([]huh.Option[string], len(c.Options))
	label := map[string]string{}
	for i, o := range c.Options {
		opts[i] = huh.NewOption(o.Label, o.Value)
		label[o.Value] = o.Label
	}
	s := huh.NewSelect[string]().Title(c.Title).Options(opts...).Value(&v)
	if len(c.Note) > 0 {
		s = s.Description(strings.Join(c.Note, "\n"))
	}
	if err := t.run(s); err != nil {
		return "", err
	}
	t.answered(c.Title, label[v])
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

// Flush shows a last line that had no newline.
func (g *gutterWriter) Flush() {
	if g.buf.Len() > 0 {
		g.Write([]byte("\n"))
	}
}
