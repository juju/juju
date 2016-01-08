// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// NewCollectMetricsCommand returns a new command that triggers the collect
// metrics hook on specified units and stores collected metrics in
// controller's database.
func NewCollectMetricsCommand() cmd.Command {
	return envcmd.Wrap(&collectMetricsCommand{})
}

type collectMetricsCommand struct {
	envcmd.EnvCommandBase
	out cmd.Output

	timeout  time.Duration
	services []string
	units    []string
}

const collectMetricsDoc = `
Collect metrics command triggers the collect metrics hook in specified units
or units of specified services and collects reported metrics in the 
controller's database.

Multiple values can be set for --service and --unit by using comma 
separated values. 

If the target is a service, metrics collection is triggered on all units
for that service.
`

// Info implements the cmd.Command interface.
func (c *collectMetricsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "collect-metrics",
		Args:    "",
		Purpose: "trigger collect metrics hook on the specified remote targets",
		Doc:     collectMetricsDoc,
	}
}

// SetFlags implements the cmd.Command interface.
func (c *collectMetricsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.DurationVar(&c.timeout, "timeout", 5*time.Minute, "how long to wait before the remote command is considered to have failed")
	f.Var(cmd.NewStringsValue(nil, &c.services), "service", "one or more service names")
	f.Var(cmd.NewStringsValue(nil, &c.units), "unit", "one or more unit ids")
}

// Init implements the cmd.Command interface.
func (c *collectMetricsCommand) Init(args []string) error {
	if len(c.services) == 0 && len(c.units) == 0 {
		return fmt.Errorf("You must specify a target, either through --service or --unit")
	}

	var nameErrors []string
	for _, service := range c.services {
		if !names.IsValidService(service) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid service name", service))
		}
	}
	for _, unit := range c.units {
		if !names.IsValidUnit(unit) {
			nameErrors = append(nameErrors, fmt.Sprintf("  %q is not a valid unit name", unit))
		}
	}
	if len(nameErrors) > 0 {
		return fmt.Errorf("The following run targets are not valid:\n%s",
			strings.Join(nameErrors, "\n"))
	}

	return cmd.CheckEmpty(args)
}

// Run implements the cmd.Command interface.
func (c *collectMetricsCommand) Run(ctx *cmd.Context) error {
	client, err := getCollectMetricsAPIClient(c)
	if err != nil {
		return err
	}

	collectParams := params.CollectMetricsParams{
		Timeout:  c.timeout,
		Services: c.services,
		Units:    c.units,
	}

	collectResults, err := client.CollectMetrics(collectParams)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	results := make([]interface{}, len(collectResults))
	for i, collectResult := range collectResults {
		values := make(map[string]interface{})
		values["UnitId"] = collectResult.UnitId
		if collectResult.Error != "" {
			values["Error"] = collectResult.Error
		}
		results[i] = values
	}

	err = c.out.Write(ctx, results)
	if err != nil {
		return err
	}

	return nil
}

// In order to be able to easily mock out the API side for testing,
// the API client is obtained using a function.

type CollectMetricsClient interface {
	Close() error
	CollectMetrics(run params.CollectMetricsParams) ([]params.CollectMetricsResult, error)
}

// Here we need the signature to be correct for the interface.
var getCollectMetricsAPIClient = func(c *collectMetricsCommand) (CollectMetricsClient, error) {
	return c.NewAPIClient()
}
