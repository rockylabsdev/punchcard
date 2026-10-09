package cli

import (
	"fmt"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// The reads: status, ls and version. They never call Jira and never
// write; status and ls exit 0 unless the state cannot be read.

func (a *app) statusCmd() *cobra.Command {
	var shortForm bool
	c := &cobra.Command{
		Use:   "status",
		Short: "Show the running ticket, paused tickets and unsent worklogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.status(shortForm)
		},
	}
	c.Flags().BoolVar(&shortForm, "short", false, "one line for a shell prompt; nothing when idle; always exits 0")
	return c
}

func (a *app) status(shortForm bool) error {
	w, now, err := a.read()
	if err != nil {
		if shortForm && !a.out.json {
			return nil // a shell prompt shows nothing and never fails
		}
		a.code = a.failure("status", err)
		if shortForm {
			a.code = ExitOK
		}
		return nil
	}
	d := snapshot(w, now)
	var human string
	if shortForm {
		human = a.out.shortLine(d)
	} else {
		human = a.out.statusText(d, now)
	}
	a.code = a.out.ok("status", d, nil, human)
	return nil
}

func (o output) statusText(d StatusData, now time.Time) string {
	var b strings.Builder
	if d.Running != nil {
		b.WriteString(o.timerLine(*d.Running, now.Location(), now))
	} else {
		b.WriteString("Nothing running.\n")
	}
	for _, tm := range d.Paused {
		b.WriteString(o.timerLine(tm, now.Location(), now))
	}
	if d.Unsent.Count > 0 {
		fmt.Fprintf(&b, "%d unsent worklog(s), %s\n", d.Unsent.Count, clock(secs(d.Unsent.Seconds)))
	}
	if !d.AuthFailedAt.IsZero() {
		fmt.Fprintf(&b, "%s Jira rejected the token at %s; run `punchcard setup`\n",
			o.paint(red, "!"), d.AuthFailedAt.In(now.Location()).Format("2006-01-02 15:04"))
	}
	return b.String()
}

// shortLine is `status --short`: ▶ ABC-123 1:12 · 2 unsent, ⏸ …, ! … stale.
// Idle with nothing owed prints nothing; idle with worklogs owed prints
// only the unsent count, which status --short always shows.
func (o output) shortLine(d StatusData) string {
	var parts []string
	var tm *Timer
	if d.Running != nil {
		tm = d.Running
	} else if len(d.Paused) > 0 {
		tm = &d.Paused[0]
	}
	if tm != nil {
		switch tm.State {
		case "running":
			parts = append(parts, fmt.Sprintf("%s %s %s", o.paint(green, "▶"), tm.Key, short(secs(tm.ElapsedSeconds))))
		case "paused":
			parts = append(parts, fmt.Sprintf("%s %s %s", o.paint(yellow, "⏸"), tm.Key, short(secs(tm.ElapsedSeconds))))
		case "stale":
			parts = append(parts, fmt.Sprintf("%s %s stale", o.paint(red, "!"), tm.Key))
		}
	}
	if d.Unsent.Count > 0 {
		parts = append(parts, fmt.Sprintf("%d unsent", d.Unsent.Count))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + "\n"
}

func (a *app) lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List tracked tickets, most recently active first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, now, err := a.read()
			if err != nil {
				a.code = a.failure("ls", err)
				return nil
			}
			d := listData(w, now, nil)
			a.code = a.out.ok("ls", d, nil, a.out.listText(d))
			return nil
		},
	}
}

func (o output) listText(d ListData) string {
	if len(d.Tickets) == 0 {
		return "No tickets yet. Add one with `punchcard add KEY` or `punchcard start KEY`.\n"
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, t := range d.Tickets {
		marker, state, elapsed := " ", "-", ""
		switch t.State {
		case "running":
			marker = o.paint(green, "▶")
		case "paused":
			marker = o.paint(yellow, "⏸")
		case "stale":
			marker = o.paint(red, "!")
		}
		if t.State != "" {
			state, elapsed = t.State, clock(secs(t.ElapsedSeconds))
		}
		unsent := ""
		if t.Unsynced > 0 {
			unsent = fmt.Sprintf("%d unsent", t.Unsynced)
		}
		summary := t.Summary
		if t.Unverified {
			summary = "(unverified)"
		}
		fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\t%s\n", marker, t.Key, state, elapsed, unsent, summary)
	}
	tw.Flush()
	return b.String()
}

func (a *app) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version, contract version and platform",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d := VersionData{
				Version:       a.env.Version,
				Contract:      ContractVersion,
				SchemaVersion: core.SchemaVersion,
				Go:            runtime.Version(),
				Platform:      runtime.GOOS + "/" + runtime.GOARCH,
			}
			a.code = a.out.ok("version", d, nil,
				fmt.Sprintf("punchcard %s (contract v%d, %s %s)\n", d.Version, d.Contract, d.Go, d.Platform))
			return nil
		},
	}
}
