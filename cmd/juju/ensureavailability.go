// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
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
	// If specified, these specific machine(s) will be used to host
	// new state servers. If there are more state servers required than
	// machines specified, new machines will be created.
	// Placement is passed verbatim to the API, to be evaluated and used server-side.
	Placement []string
	// PlacementSpec holds the unparsed placement directives argument (--to).
	PlacementSpec string
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
 juju ensure-availability -n 7 --to server1,server2 --constraints mem=8G
     Ensure that 7 state servers are available, with machines server1 and
     server2 used first, and if necessary, newly created state server
     machines having the default series, and at least 8GB RAM.
`

// formatSimple marshals value to a yaml-formatted []byte, unless value is nil.
func formatSimple(value interface{}) ([]byte, error) {
	ensureAvailabilityResult, ok := value.(availabilityInfo)
	if !ok {
		return nil, fmt.Errorf("unexpected result type for ensure-availability call")
	}

	buff := &bytes.Buffer{}

	for _, machineList := range []struct {
		message string
		list    []string
	}{
		{
			"maintaining machines: %s\n",
			ensureAvailabilityResult.Maintained,
		},
		{
			"adding machines: %s\n",
			ensureAvailabilityResult.Added,
		},
		{
			"removing machines %s\n",
			ensureAvailabilityResult.Removed,
		},
		{
			"promoting machines %s\n",
			ensureAvailabilityResult.Promoted,
		},
		{
			"demoting machines %s\n",
			ensureAvailabilityResult.Demoted,
		},
	} {
		if len(machineList.list) == 0 {
			continue
		}
		_, err := fmt.Fprintf(buff, machineList.message, strings.Join(machineList.list, ", "))
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
	f.StringVar(&c.PlacementSpec, "to", "", "the machine(s) to become state servers, bypasses constraints")
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
	if c.PlacementSpec != "" {
		placementSpecs := strings.Split(c.PlacementSpec, ",")
		c.Placement = make([]string, len(placementSpecs))
		for i, spec := range placementSpecs {
			_, err := instance.ParsePlacement(strings.TrimSpace(spec))
			if err != instance.ErrPlacementScopeMissing {
				// We only support unscoped placement directives.
				return fmt.Errorf("unsupported ensure-availability placement directive %q", spec)
			}
			c.Placement[i] = spec
		}
	}
	return cmd.CheckEmpty(args)
}

type availabilityInfo struct {
	Maintained []string `json:"maintained,omitempty" yaml:"maintained,flow,omitempty"`
	Removed    []string `json:"removed,omitempty" yaml:"removed,flow,omitempty"`
	Added      []string `json:"added,omitempty" yaml:"added,flow,omitempty"`
	Promoted   []string `json:"promoted,omitempty" yaml:"promoted,flow,omitempty"`
	Demoted    []string `json:"demoted,omitempty" yaml:"demoted,flow,omitempty"`
}

// highAvailabilityVersion returns the version of the HighAvailability facade
// available on the server.
// Override for testing.
var highAvailabilityVersion = func(root *api.State) int {
	return root.BestFacadeVersion("HighAvailability")
}

// Run connects to the environment specified on the command line
// and calls EnsureAvailability.
func (c *EnsureAvailabilityCommand) Run(ctx *cmd.Context) error {
	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Annotate(err, "cannot get API connection")
	}
	var ensureAvailabilityResult params.StateServersChanges
	// Use the new HighAvailability facade if it exists.
	if highAvailabilityVersion(root) < 1 {
		if len(c.Placement) > 0 {
			return fmt.Errorf("placement directives not supported with this version of Juju")
		}
		client := root.Client()
		defer client.Close()
		ensureAvailabilityResult, err = client.EnsureAvailability(c.NumStateServers, c.Constraints, c.Series)
	} else {
		client := highavailability.NewClient(root, root.EnvironTag())
		defer client.Close()
		ensureAvailabilityResult, err = client.EnsureAvailability(c.NumStateServers, c.Constraints, c.Series, c.Placement)
	}
	if err != nil {
		return err
	}

	result := availabilityInfo{
		Added:      machineTagsToIds(ensureAvailabilityResult.Added...),
		Removed:    machineTagsToIds(ensureAvailabilityResult.Removed...),
		Maintained: machineTagsToIds(ensureAvailabilityResult.Maintained...),
		Promoted:   machineTagsToIds(ensureAvailabilityResult.Promoted...),
		Demoted:    machineTagsToIds(ensureAvailabilityResult.Demoted...),
	}
	return c.out.Write(ctx, result)
}

// Convert machine tags to ids, skipping any non-machine tags.
func machineTagsToIds(tags ...string) []string {
	var result []string

	for _, rawTag := range tags {
		tag, err := names.ParseTag(rawTag)
		if err != nil {
			continue
		}
		result = append(result, tag.Id())
	}
	return result

}
