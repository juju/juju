// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju"
)

type EnsureAvailabilityCommand struct {
	envcmd.EnvCommandBase
	out cmd.Output

	NumStateServers int
	// If specified, use this series for newly created machines,
	// else use the environment's default-series
	Series string
	// If specified, these constraints will be merged with those
	// already in the environment when creating new machines.
	Constraints constraints.Value
}

const ensureAvailabilityDoc = `
To ensure availability of deployed services, the Juju infrastructure
must itself be highly available.  Ensure-availability must be called
to ensure that the specified number of state servers are made available.

An odd number of state servers is required.

Examples:
 juju ensure-availability
     Ensure that the system is still in highly available mode. If
     there is only 1 state server running, this will ensure there
     are 3 running. If you have previously requested more than 3,
     then that number will be ensured.
 juju ensure-availability -n 5 --series=trusty
     Ensure that 5 state servers are available, with newly created
     state server machines having the "trusty" series.
 juju ensure-availability -n 7 --constraints mem=8G
     Ensure that 7 state servers are available, with newly created
     state server machines having the default series, and at least
     8GB RAM.
`

// FormatYaml marshals value to a yaml-formatted []byte, unless value is nil.
func formatSimple(value interface{}) ([]byte, error) {
	ensureAvailabilityResult, ok := value.(formattedResult)
	if !ok {
		return nil, nil
	}

	buff := &bytes.Buffer{}

	if len(ensureAvailabilityResult.Maintained) > 0 {
		_, err := fmt.Fprintf(buff, "maintaining machines: %s\n", machineList(ensureAvailabilityResult.Maintained))
		if err != nil {
			return nil, err
		}
	}
	if len(ensureAvailabilityResult.Added) > 0 {
		_, err := fmt.Fprintf(buff, "adding machines: %s\n", machineList(ensureAvailabilityResult.Added))
		if err != nil {
			return nil, err
		}
	}
	if len(ensureAvailabilityResult.Removed) > 0 {
		_, err := fmt.Fprintf(buff, "removing machines: %s\n", machineList(ensureAvailabilityResult.Removed))
		if err != nil {
			return nil, err
		}
	}
	return buff.Bytes(), nil
}

func (c *EnsureAvailabilityCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ensure-availability",
		Purpose: "ensure the availability of Juju state servers",
		Doc:     ensureAvailabilityDoc,
	}
}

func (c *EnsureAvailabilityCommand) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&c.NumStateServers, "n", 0, "number of state servers to make available")
	f.StringVar(&c.Series, "series", "", "the charm series")
	f.Var(constraints.ConstraintsValue{&c.Constraints}, "constraints", "additional machine constraints")
	c.out.AddFlags(f, "simple", map[string]cmd.Formatter{
		"yaml":   cmd.FormatYaml,
		"json":   cmd.FormatJson,
		"simple": formatSimple,
	})

}

func (c *EnsureAvailabilityCommand) Init(args []string) error {
	if c.NumStateServers < 0 || (c.NumStateServers%2 != 1 && c.NumStateServers != 0) {
		return fmt.Errorf("must specify a number of state servers odd and non-negative")
	}
	return cmd.CheckEmpty(args)
}

type formattedResult struct {
	Maintained []string `json:"maintained,omitempty" yaml:",flow,omitempty"`
	Removed    []string `json:"removed,omitempty" yaml:",flow,omitempty"`
	Added      []string `json:"added,omitempty" yaml:",flow,omitempty"`
}

// Run connects to the environment specified on the command line
// and calls EnsureAvailability.
func (c *EnsureAvailabilityCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	ensureAvailabilityResult, err := client.EnsureAvailability(c.NumStateServers, c.Constraints, c.Series)
	if err != nil {
		return err
	}

	result := formattedResult{
		Added:      ensureAvailabilityResult.Added,
		Removed:    ensureAvailabilityResult.Removed,
		Maintained: ensureAvailabilityResult.Maintained,
	}
	return c.out.Write(ctx, result)
}

// Format machine list for presentation.
func machineList(machines []string) string {
	quoted := []string{}
	for _, machineId := range machines {
		quoted = append(quoted, fmt.Sprintf("%q", machineId))
	}
	return strings.Join(quoted, ", ")
}
