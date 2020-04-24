// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
)

// Metric represents a single metric set by the charm.
type Metric struct {
	Key    string
	Value  string
	Time   time.Time
	Labels map[string]string `json:",omitempty"`
}

// AddMetricCommand implements the add-metric command.
type AddMetricCommand struct {
	cmd.CommandBase
	ctx     Context
	Labels  string
	Metrics []Metric
}

// NewAddMetricCommand generates a new AddMetricCommand.
func NewAddMetricCommand(ctx Context) (cmd.Command, error) {
	return &AddMetricCommand{ctx: ctx}, nil
}

// Info returns the command info structure for the add-metric command.
func (c *AddMetricCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-metric",
		Args:    "key1=value1 [key2=value2 ...]",
		Purpose: "add metrics",
	})
}

// SetFlags implements Command.
func (c *AddMetricCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Labels, "l", "", "labels to be associated with metric values")
	f.StringVar(&c.Labels, "labels", "", "")
}

// Init parses the command's parameters.
func (c *AddMetricCommand) Init(args []string) error {
	// TODO(fwereade): 2016-03-17 lp:1558657
	now := time.Now()
	if len(args) == 0 {
		return fmt.Errorf("no metrics specified")
	}
	kvs, err := keyvalues.Parse(args, false)
	if err != nil {
		return errors.Annotate(err, "invalid metrics")
	}
	var labelArgs []string
	if c.Labels != "" {
		labelArgs = strings.Split(c.Labels, ",")
	}
	for key, value := range kvs {
		labels, err := keyvalues.Parse(labelArgs, false)
		if err != nil {
			return errors.Annotate(err, "invalid labels")
		}
		c.Metrics = append(c.Metrics, Metric{
			Key:    key,
			Value:  value,
			Time:   now,
			Labels: labels,
		})
	}
	return nil
}

// Run adds metrics to the hook context.
func (c *AddMetricCommand) Run(ctx *cmd.Context) (err error) {
	for _, metric := range c.Metrics {
		if charm.IsBuiltinMetric(metric.Key) {
			return errors.Errorf("%v uses a reserved prefix", metric.Key)
		}
		if len(metric.Labels) > 0 {
			err := c.ctx.AddMetricLabels(metric.Key, metric.Value, metric.Time, metric.Labels)
			if err != nil {
				return errors.Annotate(err, "cannot record metric")
			}
		} else {
			err := c.ctx.AddMetric(metric.Key, metric.Value, metric.Time)
			if err != nil {
				return errors.Annotate(err, "cannot record metric")
			}
		}
	}
	return nil
}
