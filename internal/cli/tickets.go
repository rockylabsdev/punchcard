package cli

import (
	"fmt"
	"strings"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// The list commands: add and rm.

func (a *app) addCmd() *cobra.Command {
	var summary string
	c := &cobra.Command{
		Use:   "add KEY... [--summary TEXT]",
		Short: "Put tickets on the list without starting a timer",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			typed := cmd.Flags().Changed("summary")
			switch {
			case typed && a.env.HasTracker():
				return usageError("--summary is for no tracker: with one, the summary is the tracker's")
			case typed && len(args) > 1:
				return usageError("--summary names one ticket: add one KEY with it")
			}
			c := core.Add{Origin: a.origin, Grammar: a.env.Grammar()}
			keys := make([]string, len(args))
			for i, k := range args {
				keys[i] = a.key(k)
				n, err := a.newTicket(keys[i])
				if err != nil {
					a.code = a.failure("add", err)
					return nil
				}
				if typed {
					n.Summary = strings.TrimSpace(summary)
				}
				c.Tickets = append(c.Tickets, n)
			}
			res, err := a.apply(c)
			if err != nil {
				a.code = a.failure("add", err)
				return nil
			}
			d := listData(res.World, res.Now, keys, false)
			restored := map[string]bool{}
			for _, e := range res.Events {
				restored[e.Key] = restored[e.Key] || e.Kind == core.EventRestored
			}
			var b strings.Builder
			for _, t := range d.Tickets {
				switch {
				case restored[t.Key]:
					fmt.Fprintf(&b, "+ %s restored\n", t.Key)
				case t.AddedAt.Equal(core.At(res.Now).Time):
					fmt.Fprintf(&b, "+ %s added\n", t.Key)
				default:
					fmt.Fprintf(&b, "%s is already on the list\n", t.Key)
				}
			}
			a.done("add", res, d, b.String())
			return nil
		},
	}
	c.Flags().StringVar(&summary, "summary", "", "with no tracker, the ticket's summary as it should read on the list (one KEY)")
	return c
}

func (a *app) rmCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "rm KEY",
		Short: "Take a ticket off the list; its logged worklogs stay in the log (refused while it has unlogged time)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := a.key(args[0])
			res, err := a.apply(core.Remove{Key: key, Force: force, Origin: a.origin})
			if err != nil {
				a.code = a.failure("rm", err)
				return nil
			}
			t, _ := res.World.State.Ticket(key)
			d := RemoveData{Key: key, KeptWorklogs: core.KeptWorklogs(t)}
			human := fmt.Sprintf("- %s removed\n", key)
			switch d.KeptWorklogs {
			case 0:
			case 1:
				human = fmt.Sprintf("- %s removed (its 1 worklog stays in the log)\n", key)
			default:
				human = fmt.Sprintf("- %s removed (its %d worklogs stay in the log)\n", key, d.KeptWorklogs)
			}
			a.done("rm", res, d, human)
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "remove it even with a running or paused entry or a worklog not yet sent")
	return c
}
