// PROTOTYPE — throwaway. Answers ticket #13 "TUI layout and interaction"
// (https://github.com/j-dsmith/punchcard/issues/13). Not production code:
// in-memory state only, no tests, no Jira, no state.json.
//
// Three structurally different layouts of the same TUI, switchable live:
//
//	A  Panes   — lazygit-style: ticket list | entries + worklog queue
//	B  Ledger  — one dense table, on-the-clock header strip, inline expansion, modal stop
//	C  Clock   — the running timer in block digits dominates; compact list; drawer stop
//
// Run:  cd prototype/tui-layout && go run .        (optionally: go run . -variant B)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

const (
	scNormal = iota
	scRecovered
	scAuthFailed
	scNotConfigured
	scEmpty
	scCount
)

var scenarioNames = []string{"normal day", "recovered entry", "auth failed (401)", "not configured", "first run, empty"}

var variants = []struct{ key, name string }{{"A", "Panes"}, {"A2", "Panes, refined"}, {"A3", "Panes, quiet"}, {"A4", "Panes + Jira ledger"}, {"A5", "Labelled"}, {"B", "Ledger"}, {"C", "Clock"}}

type colourMode struct {
	name    string
	profile colorprofile.Profile
	auto    bool
}

var colourModes = []colourMode{
	{"auto", 0, true},
	{"256", colorprofile.ANSI256, false},
	{"16", colorprofile.ANSI, false},
	{"none", colorprofile.Ascii, false},
	{"truecolor", colorprofile.TrueColor, false},
}

type mode int

const (
	modeList mode = iota
	modeStop
	modeAdd
	modeConfirmRemove
	modeHelp
)

type model struct {
	w, h     int
	now      time.Time
	st       *store
	scenario int
	variant  int
	colour   int
	frame    bool // clamp design area to 80×24
	hideBar  bool

	sel  int // index into st.sorted()
	mode mode

	// stop dialog
	stopT       *ticket
	durIn       textinput.Model
	comIn       textinput.Model
	stopFoc     int
	stopErr     string
	addIn       textinput.Model
	addSel      int // -1 = typing, >=0 = recents row
	addErr      string
	toast       string
	toastAt     time.Time
	syncing     bool
	extStep     int
	paneFoc     int // variant A: which pane has focus
	bannerPlace int // variant A3: F7 cycles top / bottom / border
}

type tickMsg time.Time
type syncDoneMsg struct{}
type setupDoneMsg struct{ err error }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func newModel(variant int) model {
	now := time.Now()
	m := model{now: now, variant: variant, st: seed(now, scNormal)}
	m.durIn = textinput.New()
	m.durIn.Prompt = ""
	m.durIn.SetWidth(12)
	m.comIn = textinput.New()
	m.comIn.Prompt = ""
	m.comIn.Placeholder = "optional — what did you do?"
	m.comIn.SetWidth(40)
	m.addIn = textinput.New()
	m.addIn.Prompt = ""
	m.addIn.Placeholder = "PUN-123"
	m.addIn.SetWidth(16)
	return m
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) selected() *ticket {
	ts := m.st.sorted()
	if len(ts) == 0 {
		return nil
	}
	if m.sel >= len(ts) {
		m.sel = len(ts) - 1
	}
	return ts[max(0, min(m.sel, len(ts)-1))]
}

func (m *model) flash(s string) { m.toast, m.toastAt = s, m.now }

func (m *model) selectTicket(t *ticket) {
	for i, x := range m.st.sorted() {
		if x == t {
			m.sel = i
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case syncDoneMsg:
		m.syncing = false
		ok, failed := 0, 0
		for _, t := range m.st.tickets {
			for _, e := range t.entries {
				if e.sync == syncPending || e.sync == syncFailed {
					if m.scenario == scAuthFailed {
						e.sync, e.syncErr = syncFailed, "Jira 401: token rejected"
						failed++
					} else {
						e.sync, e.syncErr, e.sentAt = syncSynced, "", m.now
						ok++
					}
				}
			}
		}
		if failed > 0 {
			m.flash(fmt.Sprintf("sync: %d failed — still owed, nothing lost", failed))
		} else {
			m.flash(fmt.Sprintf("sync: %d worklog(s) reached Jira", ok))
		}
		m.st.log(m.now, "hand", "sync: %d sent, %d failed", ok, failed)
		return m, nil
	case setupDoneMsg:
		if m.scenario == scNotConfigured || m.scenario == scAuthFailed {
			m.scenario = scNormal
		}
		m.flash("back from punchcard setup")
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	// ---- prototype controls (not part of the design) ----
	switch s {
	case "ctrl+c":
		return m, tea.Quit
	case "f2", "ctrl+n":
		m.variant = (m.variant + 1) % len(variants)
		return m, nil
	case "f3", "ctrl+o":
		m.colour = (m.colour + 1) % len(colourModes)
		return m, nil
	case "f4", "ctrl+e":
		m.scenario = (m.scenario + 1) % scCount
		m.st = seed(m.now, m.scenario)
		m.sel, m.mode = 0, modeList
		return m, nil
	case "f5", "ctrl+g":
		m.external()
		return m, nil
	case "f6", "ctrl+f":
		m.frame = !m.frame
		return m, nil
	case "f7", "ctrl+y":
		m.bannerPlace = (m.bannerPlace + 1) % len(bannerPlaces)
		return m, nil
	case "f1", "ctrl+b":
		m.hideBar = !m.hideBar
		return m, nil
	}

	switch m.mode {
	case modeStop:
		return m.keyStop(k)
	case modeAdd:
		return m.keyAdd(k)
	case modeConfirmRemove:
		if s == "y" {
			if t := m.selected(); t != nil {
				m.st.remove(t)
				m.st.log(m.now, "hand", "removed %s", t.key)
			}
		}
		m.mode = modeList
		return m, nil
	case modeHelp:
		m.mode = modeList
		return m, nil
	}

	ts := m.st.sorted()
	t := m.selected()
	switch s {
	case "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "up", "k":
		m.sel = max(0, m.sel-1)
	case "down", "j":
		m.sel = min(len(ts)-1, m.sel+1)
	case "g", "home":
		m.sel = 0
	case "G", "end":
		m.sel = len(ts) - 1
	case "tab":
		m.paneFoc = (m.paneFoc + 1) % 3
	case "enter", "space":
		if t == nil {
			return m, nil
		}
		if t.state() == stRunning {
			m.st.pauseRunning(m.now)
			m.st.log(m.now, "hand", "paused %s", t.key)
			return m, nil
		}
		verb := "started"
		if t.state() == stPaused {
			verb = "resumed"
		}
		if p := m.st.start(m.now, t); p != nil {
			m.st.log(m.now, "hand", "%s %s · paused %s", verb, t.key, p.key)
		} else {
			m.st.log(m.now, "hand", "%s %s", verb, t.key)
		}
		m.selectTicket(t)
	case "p":
		if r := m.st.pauseRunning(m.now); r != nil {
			m.st.log(m.now, "hand", "paused %s", r.key)
		}
	case "x":
		if t == nil || t.current() == nil {
			m.flash("nothing to stop on this row — space starts it")
			return m, nil
		}
		m.openStop(t)
		return m, m.durIn.Focus()
	case "a":
		m.mode, m.addSel, m.addErr = modeAdd, -1, ""
		m.addIn.Reset()
		return m, m.addIn.Focus()
	case "d":
		if t != nil {
			m.mode = modeConfirmRemove
		}
	case "r":
		p, f := m.st.owed()
		if p+f == 0 {
			m.flash("nothing to sync")
			return m, nil
		}
		if m.scenario == scNotConfigured {
			m.flash("not connected to Jira — S to set up")
			return m, nil
		}
		m.syncing = true
		return m, tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return syncDoneMsg{} })
	case "S":
		c := exec.Command("sh", "-c", `clear; printf '\n  PROTOTYPE — this is where `+"`punchcard setup`"+` runs (TUI suspended).\n\n  Press enter to return… '; read _`)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return setupDoneMsg{err} })
	}
	return m, nil
}

func (m *model) openStop(t *ticket) {
	m.mode, m.stopT, m.stopFoc, m.stopErr = modeStop, t, 0, ""
	if r := m.st.running(); r == t {
		// the clock keeps running while the dialog is open; we read now at confirm
	}
	m.durIn.SetValue(hm(t.current().elapsed(m.now)))
	m.durIn.CursorEnd()
	m.comIn.Reset()
	m.comIn.Blur()
}

func (m model) keyStop(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.mode = modeList
		m.durIn.Blur()
		m.comIn.Blur()
		return m, nil
	case "tab", "shift+tab", "down", "up":
		m.stopFoc = 1 - m.stopFoc
		if m.stopFoc == 0 {
			m.comIn.Blur()
			return m, m.durIn.Focus()
		}
		m.durIn.Blur()
		return m, m.comIn.Focus()
	case "enter":
		d, ok := parseDur(m.durIn.Value())
		if !ok || d < time.Minute {
			m.stopErr = "duration? try 1h 20m, 80m or 1:20 (at least 1m)"
			return m, nil
		}
		t := m.stopT
		m.st.stop(m.now, t, d, m.comIn.Value())
		m.mode = modeList
		m.durIn.Blur()
		m.comIn.Blur()
		if m.scenario == scNotConfigured {
			m.st.log(m.now, "hand", "stopped %s · %s pending (not connected)", t.key, hm(d))
			m.flash("stopped — worklog waits until you set up Jira")
			return m, nil
		}
		m.st.log(m.now, "hand", "stopped %s · sending %s", t.key, hm(d))
		m.syncing = true
		return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return syncDoneMsg{} })
	}
	var cmd tea.Cmd
	if m.stopFoc == 0 {
		m.durIn, cmd = m.durIn.Update(k)
	} else {
		m.comIn, cmd = m.comIn.Update(k)
	}
	m.stopErr = ""
	return m, cmd
}

func (m model) keyAdd(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.mode = modeList
		m.addIn.Blur()
		return m, nil
	case "down":
		m.addSel = min(len(m.st.recents)-1, m.addSel+1)
		return m, nil
	case "up":
		m.addSel = max(-1, m.addSel-1)
		return m, nil
	case "enter":
		var key, sum string
		if m.addSel >= 0 {
			key, sum = m.st.recents[m.addSel].key, m.st.recents[m.addSel].summary
			m.st.recents = append(m.st.recents[:m.addSel], m.st.recents[m.addSel+1:]...)
		} else {
			key = strings.ToUpper(strings.TrimSpace(m.addIn.Value()))
			if key == "" {
				return m, nil
			}
			if strings.HasPrefix(key, "XXX") {
				m.addErr = key + " not found — or not visible to this Jira account"
				return m, nil
			}
		}
		if m.st.find(key) != nil {
			m.addErr = key + " is already on your list"
			return m, nil
		}
		t := &ticket{key: key, summary: sum, activity: m.now}
		if sum == "" {
			if m.scenario == scNotConfigured || strings.Contains(key, "OFF") {
				t.unverified = true
			} else {
				t.summary = "Fetched summary for " + key
			}
		}
		m.st.tickets = append(m.st.tickets, t)
		m.st.log(m.now, "hand", "added %s", key)
		m.mode = modeList
		m.addIn.Blur()
		m.selectTicket(t)
		return m, nil
	}
	var cmd tea.Cmd
	m.addSel = -1
	m.addIn, cmd = m.addIn.Update(k)
	m.addErr = ""
	return m, cmd
}

// external simulates a change made elsewhere (CLI, an agent, the mod), which
// the real TUI would see on its 1 s stat of state.json.
func (m *model) external() {
	ts := m.st.sorted()
	if len(ts) == 0 {
		return
	}
	mark := func(t *ticket, origin, what string) {
		t.changedAt, t.changedBy, t.changedWhat = m.now, origin, what
	}
	switch m.extStep % 3 {
	case 0: // agent starts the bottom ticket
		t := ts[len(ts)-1]
		p := m.st.start(m.now, t)
		mark(t, "agent", "started")
		if p != nil {
			mark(p, "agent", "paused")
			m.st.log(m.now, "agent", "started %s · paused %s", t.key, p.key)
		} else {
			m.st.log(m.now, "agent", "started %s", t.key)
		}
	case 1: // prompt focus moves to another keyed session's ticket
		var t *ticket
		for _, x := range ts {
			if x.state() != stRunning && x.summary != "" {
				t = x
				break
			}
		}
		if t == nil {
			return
		}
		p := m.st.start(m.now, t)
		mark(t, "prompt", "focus")
		if p != nil {
			mark(p, "prompt", "paused")
		}
		m.st.log(m.now, "prompt", "focus → %s", t.key)
	case 2: // CLI stop from a shell, by hand
		r := m.st.running()
		if r == nil {
			m.extStep++
			m.external()
			return
		}
		d := r.current().elapsed(m.now).Truncate(time.Minute)
		m.st.stop(m.now, r, max(d, time.Minute), "")
		mark(r, "hand", "stopped")
		m.st.log(m.now, "hand", "punchcard stop %s · %s pending", r.key, hm(d))
	}
	m.extStep++
}

// ---- shared chrome -----------------------------------------------------------

// banner: the single, never-dismissible slot showing the worst condition.
func (m model) banner(w int) string {
	p, f := m.st.owed()
	var txt string
	st := sBannerErr
	switch {
	case m.scenario == scAuthFailed:
		txt = fmt.Sprintf(" ✗ Jira rejected your token at %s · %d worklogs waiting · S to fix", m.now.Add(-38*time.Minute).Format("15:04"), p+f)
	case m.scenario == scNotConfigured:
		txt = " Not connected to Jira — worklogs will wait. Press S to set up."
		st = sBannerWarn
	case f > 0:
		txt = fmt.Sprintf(" ✗ %d worklog failed to reach Jira · r to retry · nothing is lost", f)
	default:
		return ""
	}
	return st.Width(w).Render(ansi.Truncate(txt, w, "…"))
}

// changed reports a recent change made elsewhere, to mark the row.
func (m model) changed(t *ticket) string {
	if t.changedAt.IsZero() || m.now.Sub(t.changedAt) > 6*time.Second {
		return ""
	}
	return sChanged.Render(t.changedWhat + " by " + t.changedBy)
}

func (m model) lastEvent(w int) string {
	if m.toast != "" && m.now.Sub(m.toastAt) < 5*time.Second {
		return ansi.Truncate(" "+sBold.Render(m.toast), w, "…")
	}
	if m.syncing {
		return sSync.Render(" " + gPending + " syncing…")
	}
	if len(m.st.events) == 0 {
		return ""
	}
	e := m.st.events[len(m.st.events)-1]
	return ansi.Truncate(sDim.Render(" "+e.at.Format("15:04")+" ")+originTag(e.origin)+" "+sDim.Render(e.text), w, "…")
}

func originTag(o string) string {
	switch o {
	case "agent":
		return sChanged.Render("agent")
	case "prompt":
		return sChanged.Render("prompt")
	}
	return sDim.Render("you")
}

type binding struct{ k, label string }

// keybar: context-sensitive bindings for the current mode/selection.
func (m model) keys() []binding {
	switch m.mode {
	case modeStop:
		return []binding{{"enter", "stop & send"}, {"tab", "field"}, {"esc", "cancel — keep timer"}}
	case modeAdd:
		return []binding{{"enter", "add"}, {"↑↓", "recents"}, {"esc", "cancel"}}
	case modeConfirmRemove:
		return []binding{{"y", "remove"}, {"any", "keep"}}
	case modeHelp:
		return []binding{{"any", "close"}}
	}
	t := m.selected()
	var b []binding
	if t != nil {
		switch t.state() {
		case stRunning:
			b = append(b, binding{"space", "pause"}, binding{"x", "stop…"})
		case stPaused:
			b = append(b, binding{"space", "resume"}, binding{"x", "stop…"})
		default:
			b = append(b, binding{"space", "start"})
		}
	}
	b = append(b, binding{"a", "add"})
	if t != nil {
		b = append(b, binding{"d", "remove"})
	}
	if p, f := m.st.owed(); p+f > 0 {
		b = append(b, binding{"r", fmt.Sprintf("sync %d", p+f)})
	}
	b = append(b, binding{"S", "setup"}, binding{"?", "help"}, binding{"q", "quit"})
	return b
}

func (m model) keybar(w int) string {
	var parts []string
	for _, b := range m.keys() {
		ks := sKey.Render(b.k)
		if b.k == "r" {
			ks = sSync.Bold(true).Render(b.k)
		}
		if b.k == "x" {
			ks = sFail.Render(b.k)
		}
		parts = append(parts, ks+" "+sDim.Render(b.label))
	}
	if m.mode == modeConfirmRemove {
		t := m.selected()
		pre := sFail.Render(" remove "+t.key+"?") + " "
		if c := t.current(); c != nil {
			pre = sFail.Render(" remove "+t.key+"? its unsent "+hm(c.elapsed(m.now))+" is discarded") + " "
		}
		return ansi.Truncate(pre+strings.Join(parts, "  "), w, "…")
	}
	return ansi.Truncate(" "+strings.Join(parts, sFaint.Render("  ")), w, "…")
}

func (m model) help(w, h int) string {
	lines := []string{
		sBold.Render("Keys"),
		"",
		"  space / enter   start · pause · resume the selected ticket",
		"  p               pause whatever is running (from any row)",
		"  x               stop the selected entry → review → send to Jira",
		"  a / d           add a ticket / remove the selected one",
		"  r               sync: send pending and failed worklogs",
		"  S               run punchcard setup (TUI suspends)",
		"  j k ↑ ↓ g G     move",
		"  q               quit — timers keep running without the TUI",
		"",
		sBold.Render("Glyphs"),
		"",
		"  " + sRunning.Render(gRunning) + " running   " + sPaused.Render(gPaused) + " paused   " + sRecov.Render(gRecovered) + " recovered: check before stop   " + sDim.Render(gIdle) + " idle",
		"  " + sSync.Render(gPending) + " pending   " + sDim.Render(gSynced) + " synced   " + sFail.Render(gFailed) + " failed   " + sDim.Render(gUnverified) + " unverified key",
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cDim).Padding(0, 2).Render(strings.Join(lines, "\n"))
}

// ---- view --------------------------------------------------------------------

func (m model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("")
	}
	dw, dh := m.w, m.h
	if !m.hideBar {
		dh--
	}
	if m.frame {
		dw, dh = min(80, dw), min(24, dh)
	}
	var body string
	switch variants[m.variant].key {
	case "A":
		body = m.viewA(dw, dh)
	case "A2":
		body = m.viewA2(dw, dh)
	case "A3":
		body = m.viewA3(dw, dh)
	case "A4":
		body = m.viewA4(dw, dh)
	case "A5":
		body = m.viewA5(dw, dh)
	case "B":
		body = m.viewB(dw, dh)
	default:
		body = m.viewC(dw, dh)
	}
	if m.mode == modeHelp {
		body = overlay(body, m.help(dw, dh), dw, dh)
	}
	// force exact size so variants can't cheat on height
	body = lipgloss.NewStyle().Width(dw).Height(dh).MaxHeight(dh).MaxWidth(dw).Render(body)
	if m.frame && m.w > dw {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, sFaint.Render(strings.Repeat("┊\n", dh-1)+"┊"))
	}
	out := body
	if !m.hideBar {
		out += "\n" + m.protoBar()
	}
	if cm := colourModes[m.colour]; !cm.auto {
		var buf bytes.Buffer
		w := &colorprofile.Writer{Forward: &buf, Profile: cm.profile}
		w.WriteString(out)
		out = buf.String()
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.WindowTitle = "punchcard (prototype)"
	return v
}

func (m model) protoBar() string {
	st := lipgloss.NewStyle().Background(lipgloss.Color("#d7005f")).Foreground(lipgloss.Color("#ffffff"))
	frame := "off"
	if m.frame {
		frame = "80×24"
	}
	txt := fmt.Sprintf(" PROTOTYPE  F2 variant %s·%s  F3 colour %s  F4 scenario: %s  F5 change elsewhere  F6 frame %s  F7 banner %s  F1 hide ",
		variants[m.variant].key, variants[m.variant].name, colourModes[m.colour].name, scenarioNames[m.scenario], frame, bannerPlaces[m.bannerPlace])
	return st.Width(m.w).Render(ansi.Truncate(txt, m.w, "…"))
}

// overlay composites top centred over base.
func overlay(base, top string, w, h int) string {
	tw, th := lipgloss.Width(top), lipgloss.Height(top)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(top).X(max(0, (w-tw)/2)).Y(max(0, (h-th)/2)).Z(1),
	).Render()
}

func main() {
	v := flag.String("variant", "A5", "A, A2, A3, A4, A5, B or C")
	smoke := flag.Bool("smoke", false, "drive every variant and scenario through a key sequence headlessly")
	dump := flag.String("dump", "", "print frames at WxH (e.g. 80x24) without colour and exit; for checking layout")
	flag.Parse()
	if *smoke {
		smokeRun()
		return
	}
	if *dump != "" {
		dumpFrames(*dump)
		return
	}
	idx := 0
	for i, x := range variants {
		if strings.EqualFold(x.key, *v) {
			idx = i
		}
	}
	if _, err := tea.NewProgram(newModel(idx)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dumpFrames(size string) {
	var w, h int
	fmt.Sscanf(size, "%dx%d", &w, &h)
	for vi := range variants {
		for _, sc := range []int{scNormal, scRecovered, scAuthFailed, scEmpty} {
			m := newModel(vi)
			m.w, m.h, m.hideBar, m.colour = w, h, true, 3
			fmt.Sscanf(os.Getenv("PC_BANNER"), "%d", &m.bannerPlace)
			m.scenario, m.st = sc, seed(m.now, sc)
			show := func(what string, m model) {
				fmt.Printf("===== %s · %s · %s\n", variants[vi].key, scenarioNames[sc], what)
				fmt.Println(m.View().Content)
			}
			show("list", m)
			if sc == scRecovered {
				mm, _ := m.key(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
				show("stop dialog", mm.(model))
			}
			if sc == scNormal {
				mm, _ := m.key(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
				show("add dialog", mm.(model))
			}
		}
	}
}

func smokeRun() {
	seq := []string{"space", "j", "space", "x", "1", "tab", "h", "enter", "a", "X", "X", "-", "1", "enter", "esc",
		"a", "N", "-", "1", "enter", "r", "f5", "f5", "f5", "x", "backspace", "backspace", "enter", "esc", "p", "G", "x", "esc",
		"d", "y", "?", "k", "f3", "f3", "f3", "f3", "f6", "f1", "g", "enter", "x", "enter"}
	keyOf := func(s string) tea.KeyPressMsg {
		switch s {
		case "space":
			return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "})
		case "enter":
			return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
		case "esc":
			return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
		case "tab":
			return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
		case "backspace":
			return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
		case "f1", "f3", "f5", "f6":
			return tea.KeyPressMsg(tea.Key{Code: map[string]rune{"f1": tea.KeyF1, "f3": tea.KeyF3, "f5": tea.KeyF5, "f6": tea.KeyF6}[s]})
		}
		r := []rune(s)[0]
		return tea.KeyPressMsg(tea.Key{Code: r, Text: s})
	}
	n := 0
	for vi := range variants {
		for sc := 0; sc < scCount; sc++ {
			for _, size := range [][2]int{{80, 25}, {120, 40}, {60, 20}} {
				m := newModel(vi)
				m.w, m.h = size[0], size[1]
				m.scenario, m.st = sc, seed(m.now, sc)
				var mm tea.Model = m
				for _, k := range seq {
					mm, _ = mm.Update(keyOf(k))
					mm, _ = mm.Update(syncDoneMsg{})
					v := mm.View().Content
					if got := lipgloss.Height(v); got != size[1] {
						fmt.Printf("height %d != %d: variant %s scenario %d size %v after %q\n", got, size[1], variants[vi].key, sc, size, k)
					}
					n++
				}
			}
		}
	}
	fmt.Println("smoke ok, frames:", n)
}
