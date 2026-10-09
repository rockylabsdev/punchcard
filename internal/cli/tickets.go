package cli

import (
	"fmt"
	"strings"

	"github.com/j-dsmith/punchcard/internal/core"
	"github.com/spf13/cobra"
)

// The list commands: add and rm.

func (a *app) addCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add KEY...",
		Short: "Put tickets on the list without starting a timer",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := core.Add{Origin: a.origin}
			for _, k := range args {
				n, err := a.newTicket(k)
				if err != nil {
					a.code = a.failure("add", err)
					return nil
				}
				c.Tickets = append(c.Tickets, n)
			}
			res, err := a.apply(c)
			if err != nil {
				a.code = a.failure("add", err)
				return nil
			}
			d := listData(res.World, res.Now, args)
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
}

func (a *app) rmCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "rm KEY",
		Short: "Take a ticket off the list (refused while it has unlogged time)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := a.apply(core.Remove{Key: args[0], Force: force, Origin: a.origin})
			if err != nil {
				a.code = a.failure("rm", err)
				return nil
			}
			a.done("rm", res, RemoveData{Key: args[0]}, fmt.Sprintf("- %s removed\n", args[0]))
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "remove it even with a running or paused entry or an unsent worklog")
	return c
}
