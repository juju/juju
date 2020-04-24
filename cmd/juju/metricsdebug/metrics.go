// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/metricsdebug"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const metricsDoc = `
Display recently collected metrics.
`

// MetricsCommand retrieves metrics stored in the juju controller.
type MetricsCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	Tags []string
	All  bool
}

// New creates a new MetricsCommand.
func New() cmd.Command {
	return modelcmd.Wrap(&MetricsCommand{})
}

// Info implements Command.Info.
func (c *MetricsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "metrics",
		Args:    "[tag1[...tagN]]",
		Purpose: "Retrieve metrics collected by specified entities.",
		Doc:     metricsDoc,
	})
}

// Init reads and verifies the cli arguments for the MetricsCommand
func (c *MetricsCommand) Init(args []string) error {
	if !c.All && len(args) == 0 {
		return errors.New("you need to specify at least one unit or application")
	} else if c.All && len(args) > 0 {
		return errors.New("cannot use --all with additional entities")
	}
	c.Tags = make([]string, len(args))
	for i, arg := range args {
		if names.IsValidUnit(arg) {
			c.Tags[i] = names.NewUnitTag(arg).String()
		} else if names.IsValidApplication(arg) {
			c.Tags[i] = names.NewApplicationTag(arg).String()
		} else {
			return errors.Errorf("%q is not a valid unit or application", args[0])
		}
	}
	return nil
}

// SetFlags implements cmd.Command.SetFlags.
func (c *MetricsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": formatTabular,
		"json":    cmd.FormatJson,
		"yaml":    cmd.FormatYaml,
	})
	f.BoolVar(&c.All, "all", false, "retrieve metrics collected by all units in the model")
}

type GetMetricsClient interface {
	GetMetrics(tags ...string) ([]params.MetricResult, error)
	Close() error
}

var newClient = func(env modelcmd.ModelCommandBase) (GetMetricsClient, error) {
	state, err := env.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return metricsdebug.NewClient(state), nil
}

type metricSlice []metric

// Len implements the sort.Interface.
func (slice metricSlice) Len() int {
	return len(slice)
}

// Less implements the sort.Interface.
func (slice metricSlice) Less(i, j int) bool {
	if slice[i].Metric == slice[j].Metric {
		return renderLabels(slice[i].Labels) < renderLabels(slice[j].Labels)
	}
	return slice[i].Metric < slice[j].Metric
}

// Swap implements the sort.Interface.
func (slice metricSlice) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type metric struct {
	Unit      string            `json:"unit" yaml:"unit"`
	Timestamp time.Time         `json:"timestamp" yaml:"timestamp"`
	Metric    string            `json:"metric" yaml:"metric"`
	Value     string            `json:"value" yaml:"value"`
	Labels    map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Run implements Command.Run.
func (c *MetricsCommand) Run(ctx *cmd.Context) error {
	client, err := newClient(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	var metrics []params.MetricResult
	if c.All {
		metrics, err = client.GetMetrics()
	} else {
		metrics, err = client.GetMetrics(c.Tags...)
	}
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	if len(metrics) == 0 {
		return nil
	}
	results := make([]metric, len(metrics))
	for i, m := range metrics {
		results[i] = metric{
			Unit:      m.Unit,
			Timestamp: m.Time,
			Metric:    m.Key,
			Value:     m.Value,
			Labels:    m.Labels,
		}
	}
	sortedResults := metricSlice(results)
	sort.Sort(sortedResults)

	return errors.Trace(c.out.Write(ctx, results))
}

// formatTabular returns a tabular view of collected metrics.
func formatTabular(writer io.Writer, value interface{}) error {
	metrics, ok := value.([]metric)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", metrics, value)
	}
	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{1, 2, 3, 4} {
		table.RightAlign(col)
	}
	table.AddRow("UNIT", "TIMESTAMP", "METRIC", "VALUE", "LABELS")
	for _, m := range metrics {
		table.AddRow(m.Unit, m.Timestamp.Format(time.RFC3339), m.Metric, m.Value, renderLabels(m.Labels))
	}
	_, err := fmt.Fprint(writer, table.String())
	return errors.Trace(err)
}

func renderLabels(m map[string]string) string {
	var result []string
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(result)
	return strings.Join(result, ",")
}
