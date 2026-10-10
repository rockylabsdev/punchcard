package cli

import (
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// LogRow is one worklog in `log`: a stopped entry, on the day it started.
type LogRow struct {
	Date      string            `json:"date"` // YYYY-MM-DD, local
	Started   core.Timestamp    `json:"started"`
	Key       string            `json:"key"`
	Summary   string            `json:"summary"`
	Minutes   int               `json:"minutes"`
	Comment   string            `json:"comment"`
	State     core.WorklogState `json:"state"`
	RemoteID  string            `json:"remoteId"`
	EntryID   string            `json:"entryId"`
	Recovered bool              `json:"recovered"`
}

// LogRange is the inclusive range of local dates `log` covered.
type LogRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// LogData is `data` for log: the rows, the open entry (null when none;
// it has no worklog yet, so it is never a row) and the range.
type LogData struct {
	Rows  []LogRow `json:"rows"`
	Open  *Timer   `json:"open"`
	Range LogRange `json:"range"`
}

const dateLayout = "2006-01-02"

// logFlags are log's range and format flags as given.
type logFlags struct {
	week, month, md, csv bool
	from, to             string
}

func (a *app) logCmd() *cobra.Command {
	var f logFlags
	c := &cobra.Command{
		Use:   "log",
		Short: "List worklogs over a range as Markdown (the default), CSV or JSON",
		Long: "List one row per stopped entry whose worklog day (the local date it started) is in the range.\n" +
			"The default range, and --week, is Monday of this week through today; --month is the first of the\n" +
			"month through today; --from and --to are inclusive YYYY-MM-DD dates, either may stand alone:\n" +
			"--from alone runs through today, --to alone starts on the Monday of its own week.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			now := a.env.Now()
			from, to, err := logRange(f, now)
			if err != nil {
				return err
			}
			format, err := formatOf(f, cmd.Flags().Changed("json"))
			if err != nil {
				return err
			}
			w, _, err := a.read()
			if err != nil {
				a.code = a.failure("log", err)
				return nil
			}
			d := logData(w, now, from, to)
			switch format {
			case logCSVOut:
				a.code = a.raw(logCSV(d))
			case logMDOut:
				a.code = a.raw(logMarkdown(d))
			default:
				a.code = a.out.ok("log", d, nil, logMarkdown(d))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&f.week, "week", false, "Monday of this week through today (the default)")
	c.Flags().BoolVar(&f.month, "month", false, "the first of this month through today")
	c.Flags().StringVar(&f.from, "from", "", "first day, YYYY-MM-DD (alone: through today, or just that day if still to come)")
	c.Flags().StringVar(&f.to, "to", "", "last day, YYYY-MM-DD (alone: from the Monday of its week)")
	c.Flags().BoolVar(&f.md, "md", false, "print Markdown: a table per day with totals (the default)")
	c.Flags().BoolVar(&f.csv, "csv", false, "print CSV with a header row, for a spreadsheet or another tracker")
	return c
}

// raw prints a format the user asked for by name on stdout, even when
// PUNCHCARD_JSON=1 would otherwise make it an envelope.
func (a *app) raw(text string) int {
	fmt.Fprint(a.env.Stdout, text)
	return ExitOK
}

// logFormat is how log prints: the envelope with --json, else the
// Markdown (logDefault), or a format asked for by name.
type logFormat string

const (
	logDefault logFormat = ""
	logCSVOut  logFormat = "csv"
	logMDOut   logFormat = "md"
)

// countTrue is how many of bs are set.
func countTrue(bs ...bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}

// midnight is the start of t's local date, in t's zone.
func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// formatOf is the format the flags name. Asking for two formats by flag
// is usage; PUNCHCARD_JSON=1 alone yields to a format flag.
func formatOf(f logFlags, jsonFlag bool) (logFormat, error) {
	switch {
	case countTrue(f.md, f.csv, jsonFlag) > 1:
		return "", usageError("--md, --csv and --json are different formats: pass one")
	case f.csv:
		return logCSVOut, nil
	case f.md:
		return logMDOut, nil
	}
	return logDefault, nil
}

// logRange is the inclusive range of local dates, as midnights in now's
// zone. A date flag alone takes its missing edge from the date given, so
// that a lone flag never makes a range the user cannot see: --to alone
// starts on the Monday of --to's week; --from alone runs through today,
// or is that one day when it is still to come. Only both flags can be
// out of order.
func logRange(f logFlags, now time.Time) (from, to time.Time, err error) {
	loc := now.Location()
	today := midnight(now)
	dates := f.from != "" || f.to != ""
	switch {
	case countTrue(f.week, f.month, dates) > 1:
		return from, to, usageError("--week, --month and --from/--to are different ranges: pass one")
	case f.month:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), today, nil
	case !dates:
		return mondayOf(today), today, nil
	}
	if f.from != "" {
		if from, err = time.ParseInLocation(dateLayout, f.from, loc); err != nil {
			return from, to, usageError(fmt.Sprintf("--from %q is not a YYYY-MM-DD date", f.from))
		}
	}
	if f.to != "" {
		if to, err = time.ParseInLocation(dateLayout, f.to, loc); err != nil {
			return from, to, usageError(fmt.Sprintf("--to %q is not a YYYY-MM-DD date", f.to))
		}
	}
	switch {
	case f.from == "":
		from = mondayOf(to)
	case f.to == "":
		to = today
		if from.After(today) {
			to = from
		}
	case from.After(to):
		return from, to, usageError(fmt.Sprintf("--from %s is after --to %s", from.Format(dateLayout), to.Format(dateLayout)))
	}
	return from, to, nil
}

// mondayOf is the Monday on or before the midnight day.
func mondayOf(day time.Time) time.Time {
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// logData is the worklogs whose day is in [from, to], by day then start,
// and the open entry: the running one, else the most recently paused.
func logData(w core.World, now time.Time, from, to time.Time) LogData {
	loc := now.Location()
	d := LogData{Rows: []LogRow{}, Range: LogRange{From: from.Format(dateLayout), To: to.Format(dateLayout)}}
	for _, t := range w.State.TrackedTickets {
		for _, e := range t.Entries {
			if !e.Stopped() {
				continue
			}
			// A worklog belongs wholly to the local date it started, even
			// when its entry ran past midnight.
			start := e.Worklog.StartedAt.In(loc)
			day := midnight(start)
			if day.Before(from) || day.After(to) {
				continue
			}
			wl := e.Worklog
			d.Rows = append(d.Rows, LogRow{
				Date: day.Format(dateLayout), Started: core.Timestamp{Time: start}, Key: t.Key, Summary: t.Summary,
				Minutes: wl.TimeSpentSeconds / 60, Comment: wl.Comment, State: wl.State, RemoteID: wl.WorklogID,
				EntryID: e.ID, Recovered: wl.Recovered,
			})
		}
	}
	slices.SortStableFunc(d.Rows, func(x, y LogRow) int { return x.Started.Compare(y.Started.Time) })
	t, e, ok := core.Running(w.State)
	if !ok {
		t, e, ok = core.LastPaused(w.State)
	}
	if ok {
		tm := timerOf(w, t, e, now)
		d.Open = &tm
	}
	return d
}

// logCSV is RFC 4180 (CRLF line ends, quoted where needed) with a header,
// in columns another tracker or a spreadsheet can import: started as
// local HH:MM, whole minutes, empty strings for absent values.
func logCSV(d LogData) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	_ = w.Write([]string{"date", "started", "key", "summary", "minutes", "comment", "state", "remote_id", "entry_id", "recovered"})
	for _, r := range d.Rows {
		_ = w.Write([]string{r.Date, r.Started.Format("15:04"), r.Key, r.Summary, strconv.Itoa(r.Minutes), r.Comment,
			string(r.State), r.RemoteID, r.EntryID, strconv.FormatBool(r.Recovered)})
	}
	w.Flush() // a strings.Builder never fails to take a write
	return b.String()
}

const mdTableHead = "| key | summary | time | comment |\n|---|---|---|---|\n"

// logMarkdown is a timesheet to paste into a PR, a standup or an
// invoice: per day a heading with the day's total and any debt, then a
// table; the total; and the open entry, which is not logged yet. An
// empty range is one empty table, so the shape never surprises.
func logMarkdown(d LogData) string {
	var b strings.Builder
	total := 0
	if len(d.Rows) == 0 {
		b.WriteString(mdTableHead + "\n")
	}
	for i := 0; i < len(d.Rows); {
		j := i
		minutes, pending, failed := 0, 0, 0
		for ; j < len(d.Rows) && d.Rows[j].Date == d.Rows[i].Date; j++ {
			minutes += d.Rows[j].Minutes
			switch d.Rows[j].State {
			case core.WorklogPending:
				pending++
			case core.WorklogFailed:
				failed++
			}
		}
		date, _ := time.Parse(dateLayout, d.Rows[i].Date)
		fmt.Fprintf(&b, "## %s · %s", date.Format("2006-01-02 Mon"), hoursMinutes(minutes))
		if pending > 0 {
			fmt.Fprintf(&b, " · %d pending", pending)
		}
		if failed > 0 {
			fmt.Fprintf(&b, " · %d failed", failed)
		}
		b.WriteString("\n\n" + mdTableHead)
		for _, r := range d.Rows[i:j] {
			comment := r.Comment
			if r.State == core.WorklogPending || r.State == core.WorklogFailed {
				comment = strings.TrimPrefix(comment+" · "+string(r.State), " · ")
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", mdCell(r.Key), mdCell(r.Summary),
				short(time.Duration(r.Minutes)*time.Minute), mdCell(comment))
		}
		b.WriteString("\n")
		total += minutes
		i = j
	}
	fmt.Fprintf(&b, "**Total: %s**\n", hoursMinutes(total))
	if o := d.Open; o != nil {
		word := "Running"
		if o.State == core.TimerPaused {
			word = "Paused"
		}
		fmt.Fprintf(&b, "\n%s: %s %s, not logged\n", word, o.Key, short(secs(o.ElapsedSeconds)))
	}
	return b.String()
}

// mdCell keeps a table cell one cell on one line: pipes are escaped and
// line breaks become <br>, which GitHub renders inside a table.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "<br>")
}

// hoursMinutes formats whole minutes as 6h 05m.
func hoursMinutes(minutes int) string {
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}
