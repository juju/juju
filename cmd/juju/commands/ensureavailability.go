// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

// EnsureAvailabilityCommand makes the system highly available.
type EnsureAvailabilityCommand struct {
	envcmd.EnvCommandBase
	out      cmd.Output
	haClient EnsureAvailabilityClient

	// NumStateServers specifies the number of state servers to make available.
	NumStateServers int
	// Series is used for newly created machines, if specified.
	// Otherwise,  the environment's default-series is used.
	Series string
	// Constraints, if specified, will be merged with those already
	// in the environment when creating new machines.
	Constraints constraints.Value
	// Placement specifies specific machine(s) which will be used to host
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
		return nil, fmt.Errorf("unexpected result type for ensure-availability call: %T", value)
	}

	var buf bytes.Buffer

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
			"removing machines: %s\n",
			ensureAvailabilityResult.Removed,
		},
		{
			"promoting machines: %s\n",
			ensureAvailabilityResult.Promoted,
		},
		{
			"demoting machines: %s\n",
			ensureAvailabilityResult.Demoted,
		},
		{
			"converting machines: %s\n",
			ensureAvailabilityResult.Converted,
		},
	} {
		if len(machineList.list) == 0 {
			continue
		}
		_, err := fmt.Fprintf(&buf, machineList.message, strings.Join(machineList.list, ", "))
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (c *EnsureAvailabilityCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "ensure-availability",
		Purpose: "ensure that sufficient state servers exist to provide redundancy",
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
			p, err := instance.ParsePlacement(strings.TrimSpace(spec))
			if err == nil && names.IsContainerMachine(p.Directive) {
				return errors.New("ensure-availability cannot be used with container placement directives")
			}
			if err == nil && p.Scope == instance.MachineScope {
				// Targeting machines is ok.
				c.Placement[i] = p.String()
				continue
			}
			if err != instance.ErrPlacementScopeMissing {
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
	Converted  []string `json:"converted,omitempty" yaml:"converted,flow,omitempty"`
}

// EnsureAvailabilityClient defines the methods
// on the client api that the ensure availability
// command calls.
type EnsureAvailabilityClient interface {
	Close() error
	EnsureAvailability(
		numStateServers int, cons constraints.Value, series string,
		placement []string) (params.StateServersChanges, error)
}

func (c *EnsureAvailabilityCommand) getHAClient() (EnsureAvailabilityClient, error) {
	if c.haClient != nil {
		return c.haClient, nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get API connection")
	}

	// NewClient does not return an error, so we'll return nil
	return highavailability.NewClient(root), nil
}

// Run connects to the environment specified on the command line
// and calls EnsureAvailability.
func (c *EnsureAvailabilityCommand) Run(ctx *cmd.Context) error {
	haClient, err := c.getHAClient()
	if err != nil {
		return err
	}

	defer haClient.Close()
	ensureAvailabilityResult, err := haClient.EnsureAvailability(
		c.NumStateServers,
		c.Constraints,
		c.Series,
		c.Placement,
	)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	result := availabilityInfo{
		Added:      machineTagsToIds(ensureAvailabilityResult.Added...),
		Removed:    machineTagsToIds(ensureAvailabilityResult.Removed...),
		Maintained: machineTagsToIds(ensureAvailabilityResult.Maintained...),
		Promoted:   machineTagsToIds(ensureAvailabilityResult.Promoted...),
		Demoted:    machineTagsToIds(ensureAvailabilityResult.Demoted...),
		Converted:  machineTagsToIds(ensureAvailabilityResult.Converted...),
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
