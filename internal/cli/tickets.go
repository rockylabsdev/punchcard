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
			d := listData(res.World, res.Now, keys)
			var b strings.Builder
			for _, t := range d.Tickets {
				if t.AddedAt.Equal(core.At(res.Now).Time) {
					fmt.Fprintf(&b, "+ %s added\n", t.Key)
				} else {
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
		Short: "Take a ticket off the list (refused while it has unlogged time)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := a.key(args[0])
			res, err := a.apply(core.Remove{Key: key, Force: force, Origin: a.origin})
			if err != nil {
				a.code = a.failure("rm", err)
				return nil
			}
			a.done("rm", res, RemoveData{Key: key}, fmt.Sprintf("- %s removed\n", key))
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "remove it even with a running or paused entry or a worklog not yet sent")
	return c
}
