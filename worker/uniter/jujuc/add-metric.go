// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// builtinMetricPrefix is used to mark built-in metrics.
const builtinMetricPrefix string = "juju-"

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
		key := parts[0]
		value := parts[1]
		if strings.HasPrefix(key, builtinMetricPrefix) {
			return fmt.Errorf("cannot use add-metric to set a built-in metric %q", key)
		}
		if _, ok := keys[key]; ok {
			return fmt.Errorf("duplicate metric key given: %q", key)
		}
		keys[key] = struct{}{}
		c.Metrics = append(c.Metrics, Metric{key, value, now})
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
