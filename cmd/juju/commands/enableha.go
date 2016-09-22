// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
)

func newEnableHACommand() cmd.Command {
	haCommand := &enableHACommand{}
	haCommand.newHAClientFunc = func() (MakeHAClient, error) {
		root, err := haCommand.NewAPIRoot()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get API connection")
		}

		// NewClient does not return an error, so we'll return nil
		return highavailability.NewClient(root), nil
	}
	return modelcmd.Wrap(haCommand)
}

// enableHACommand makes the controller highly available.
type enableHACommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	// newHAClientFunc returns HA Client to be used by the command.
	newHAClientFunc func() (MakeHAClient, error)

	// NumControllers specifies the number of controllers to make available.
	NumControllers int

	// Constraints, if specified, will be merged with those already
	// in the environment when creating new machines.
	Constraints constraints.Value

	// ConstraintsStr contains the stringified version of the constraints.
	ConstraintsStr string

	// Placement specifies specific machine(s) which will be used to host
	// new controllers. If there are more controllers required than
	// machines specified, new machines will be created.
	// Placement is passed verbatim to the API, to be evaluated and used server-side.
	Placement []string

	// PlacementSpec holds the unparsed placement directives argument (--to).
	PlacementSpec string
}

const enableHADoc = `
To ensure availability of deployed applications, the Juju infrastructure
must itself be highly available.  enable-ha must be called
to ensure that the specified number of controllers are made available.

An odd number of controllers is required.

Examples:
    # Ensure that the controller is still in highly available mode. If
    # there is only 1 controller running, this will ensure there
    # are 3 running. If you have previously requested more than 3,
    # then that number will be ensured.
    juju enable-ha

    # Ensure that 5 controllers are available.
    juju enable-ha -n 5 

    # Ensure that 7 controllers are available, with newly created
    # controller machines having at least 8GB RAM.
    juju enable-ha -n 7 --constraints mem=8G

    # Ensure that 7 controllers are available, with machines server1 and
    # server2 used first, and if necessary, newly created controller
    # machines having at least 8GB RAM.
    juju enable-ha -n 7 --to server1,server2 --constraints mem=8G
`

// formatSimple marshals value to a yaml-formatted []byte, unless value is nil.
func formatSimple(writer io.Writer, value interface{}) error {
	enableHAResult, ok := value.(availabilityInfo)
	if !ok {
		return errors.Errorf("unexpected result type for enable-ha call: %T", value)
	}

	for _, machineList := range []struct {
		message string
		list    []string
	}{
		{
			"maintaining machines: %s\n",
			enableHAResult.Maintained,
		},
		{
			"adding machines: %s\n",
			enableHAResult.Added,
		},
		{
			"removing machines: %s\n",
			enableHAResult.Removed,
		},
		{
			"promoting machines: %s\n",
			enableHAResult.Promoted,
		},
		{
			"demoting machines: %s\n",
			enableHAResult.Demoted,
		},
		{
			"converting machines: %s\n",
			enableHAResult.Converted,
		},
	} {
		if len(machineList.list) == 0 {
			continue
		}
		_, err := fmt.Fprintf(writer, machineList.message, strings.Join(machineList.list, ", "))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *enableHACommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "enable-ha",
		Purpose: "Ensure that sufficient controllers exist to provide redundancy.",
		Doc:     enableHADoc,
	}
}

func (c *enableHACommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.IntVar(&c.NumControllers, "n", 0, "Number of controllers to make available")
	f.StringVar(&c.PlacementSpec, "to", "", "The machine(s) to become controllers, bypasses constraints")
	f.StringVar(&c.ConstraintsStr, "constraints", "", "Additional machine constraints")
	c.out.AddFlags(f, "simple", map[string]cmd.Formatter{
		"yaml":   cmd.FormatYaml,
		"json":   cmd.FormatJson,
		"simple": formatSimple,
	})

}

func (c *enableHACommand) Init(args []string) error {
	if c.NumControllers < 0 || (c.NumControllers%2 != 1 && c.NumControllers != 0) {
		return errors.Errorf("must specify a number of controllers odd and non-negative")
	}
	if c.PlacementSpec != "" {
		placementSpecs := strings.Split(c.PlacementSpec, ",")
		c.Placement = make([]string, len(placementSpecs))
		for i, spec := range placementSpecs {
			p, err := instance.ParsePlacement(strings.TrimSpace(spec))
			if err == nil && names.IsContainerMachine(p.Directive) {
				return errors.New("enable-ha cannot be used with container placement directives")
			}
			if err == nil && p.Scope == instance.MachineScope {
				// Targeting machines is ok.
				c.Placement[i] = p.String()
				continue
			}
			if err != instance.ErrPlacementScopeMissing {
				return errors.Errorf("unsupported enable-ha placement directive %q", spec)
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

// MakeHAClient defines the methods
// on the client api that the ensure availability
// command calls.
type MakeHAClient interface {
	Close() error
	EnableHA(
		numControllers int, cons constraints.Value,
		placement []string) (params.ControllersChanges, error)
}

// Run connects to the environment specified on the command line
// and calls EnableHA.
func (c *enableHACommand) Run(ctx *cmd.Context) error {
	var err error
	c.Constraints, err = common.ParseConstraints(ctx, c.ConstraintsStr)
	if err != nil {
		return err
	}
	haClient, err := c.newHAClientFunc()
	if err != nil {
		return err
	}

	defer haClient.Close()
	enableHAResult, err := haClient.EnableHA(
		c.NumControllers,
		c.Constraints,
		c.Placement,
	)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	result := availabilityInfo{
		Added:      machineTagsToIds(enableHAResult.Added...),
		Removed:    machineTagsToIds(enableHAResult.Removed...),
		Maintained: machineTagsToIds(enableHAResult.Maintained...),
		Promoted:   machineTagsToIds(enableHAResult.Promoted...),
		Demoted:    machineTagsToIds(enableHAResult.Demoted...),
		Converted:  machineTagsToIds(enableHAResult.Converted...),
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
