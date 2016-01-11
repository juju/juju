// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/metricsdebug"
	"github.com/juju/juju/cmd/envcmd"
)

const debugMetricsDoc = `
debug-metrics [options]
display recently collected metrics and exit
`

// DebugMetricsCommand retrieves metrics stored in the juju controller.
type DebugMetricsCommand struct {
	envcmd.EnvCommandBase
	out    cmd.Output
	Entity string
	Count  int
}

// New creates a new DebugMetricsCommand.
func New() cmd.Command {
	return envcmd.Wrap(&DebugMetricsCommand{})

}

// Info implements Command.Info.
func (c *DebugMetricsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-metrics",
		Args:    "[service or unit]",
		Purpose: "retrieve metrics collected by the given unit/service",
		Doc:     debugMetricsDoc,
	}
}

// Init reads and verifies the cli arguments for the DebugMetricsCommand
func (c *DebugMetricsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("you need to specify a unit or service.")
	}
	c.Entity = args[0]
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Errorf("unknown command line arguments: " + strings.Join(args, ","))
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *DebugMetricsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.Count, "n", 0, "number of metrics to retrieve")
}

// Run implements Command.Run.
func (c *DebugMetricsCommand) Run(ctx *cmd.Context) (rErr error) {
	state, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := metricsdebug.NewClient(state)
	metrics, err := client.GetMetrics(c.Entity)
	if err != nil {
		return errors.Trace(err)
	}
	if len(metrics) == 0 {
		return nil
	}
	if c.Count > 0 && len(metrics) > c.Count {
		metrics = metrics[:c.Count]
	}
	tw := tabwriter.NewWriter(ctx.Stdout, 0, 1, 1, ' ', 0)
	fmt.Fprintf(tw, "TIME\tMETRIC\tVALUE\n")
	for _, m := range metrics {
		fmt.Fprintf(tw, "%v\t%v\t%v\n", m.Time.Format(time.RFC3339), m.Key, m.Value)
	}
	tw.Flush()
	return nil
}
