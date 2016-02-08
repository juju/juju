// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/metricsdebug"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

const debugMetricsDoc = `
debug-metrics
display recently collected metrics and exit
`

// DebugMetricsCommand retrieves metrics stored in the juju controller.
type DebugMetricsCommand struct {
	modelcmd.ModelCommandBase
	Json  bool
	Tag   names.Tag
	Count int
}

// New creates a new DebugMetricsCommand.
func New() cmd.Command {
	return modelcmd.Wrap(&DebugMetricsCommand{})
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
	if names.IsValidUnit(args[0]) {
		c.Tag = names.NewUnitTag(args[0])
	} else if names.IsValidService(args[0]) {
		c.Tag = names.NewServiceTag(args[0])
	} else {
		return errors.Errorf("%q is not a valid unit or service", args[0])
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Errorf("unknown command line arguments: " + strings.Join(args, ","))
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *DebugMetricsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.IntVar(&c.Count, "n", 0, "number of metrics to retrieve")
	f.BoolVar(&c.Json, "json", false, "output metrics as json")
}

type GetMetricsClient interface {
	GetMetrics(tag string) ([]params.MetricResult, error)
	Close() error
}

var newClient = func(env modelcmd.ModelCommandBase) (GetMetricsClient, error) {
	state, err := env.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return metricsdebug.NewClient(state), nil
}

// Run implements Command.Run.
func (c *DebugMetricsCommand) Run(ctx *cmd.Context) error {
	client, err := newClient(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	metrics, err := client.GetMetrics(c.Tag.String())
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	if len(metrics) == 0 {
		return nil
	}
	if c.Count > 0 && len(metrics) > c.Count {
		metrics = metrics[:c.Count]
	}
	if c.Json {
		b, err := json.MarshalIndent(metrics, "", "    ")
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stdout, string(b))
		return nil
	}
	tw := tabwriter.NewWriter(ctx.Stdout, 0, 1, 1, ' ', 0)
	fmt.Fprintf(tw, "TIME\tMETRIC\tVALUE\n")
	for _, m := range metrics {
		fmt.Fprintf(tw, "%v\t%v\t%v\n", m.Time.Format(time.RFC3339), m.Key, m.Value)
	}
	tw.Flush()
	return nil
}
