// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// Metric represents a single metric set by the charm.
type Metric struct {
	Key   string
	Value string
	Time  time.Time
}

// AddMetricCommand implements the add-metric command.
type AddMetricCommand struct {
	cmd.CommandBase
	ctx     Context
	Metrics []Metric
}

// NewAddMetricCommand generates a new AddMetricCommand.
func NewAddMetricCommand(ctx Context) cmd.Command {
	return &AddMetricCommand{ctx: ctx}
}

// Info returns the command infor structure for the add-metric command.
func (c *AddMetricCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-metric",
		Args:    "key1=value1 [key2=value2 ...]",
		Purpose: "send metrics",
	}
}

// Init parses the command's parameters.
func (c *AddMetricCommand) Init(args []string) error {
	now := time.Now()
	if len(args) == 0 {
		return fmt.Errorf("no metrics specified")
	}
	keys := make(map[string]struct{})
	for _, kv := range args {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			return fmt.Errorf(`expected "key=value", got %q`, kv)
		}
		if _, ok := keys[parts[0]]; ok {
			return fmt.Errorf("duplicate metric key given: %q",
				parts[0])
		}
		keys[parts[0]] = struct{}{}
		_, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return fmt.Errorf("invalid value type: expected float, got %q", parts[1])
		}
		c.Metrics = append(c.Metrics, Metric{parts[0], parts[1], now})
	}
	return nil
}

// Run adds metrics to the hook context.
func (c *AddMetricCommand) Run(ctx *cmd.Context) (err error) {
	for _, metric := range c.Metrics {
		err := c.ctx.AddMetrics(metric.Key, metric.Value, metric.Time)
		if err != nil {
			return errors.Annotate(err, "cannot record metric")
		}
	}
	return nil
}
