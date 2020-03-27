// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewAddCommand returns a command used to add a network space.
func NewShowSpaceCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&ShowSpaceCommand{})
}

// ShowSpaceCommand calls the API to add a new network space.
type ShowSpaceCommand struct {
	SpaceCommandBase
	Name string

	out cmd.Output
}

const ShowSpaceCommandDoc = `
Displays extended information about a given space. 
Output includes the space subnets, applications with bindings to the space,
and a count of machines connected to the space.

Examples:

Show a space by name:
	juju show-space alpha

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	remove-space
`

// SetFlags implements part of the cmd.Command interface.
func (c *ShowSpaceCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Info is defined on the cmd.Command interface.
func (c *ShowSpaceCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-space",
		Args:    "<name>",
		Purpose: "Shows information about the network space.",
		Doc:     strings.TrimSpace(ShowSpaceCommandDoc),
	})
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *ShowSpaceCommand) Init(args []string) error {
	if lArgs := len(args); lArgs != 1 {
		return errors.Errorf("expected exactly 1 space name, got %d arguments", lArgs)
	}
	c.Name = args[0]
	return nil
}

// Run implements Command.Run.
func (c *ShowSpaceCommand) Run(ctx *cmd.Context) error {
	return c.RunWithSpaceAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		// Add the new space.
		space, err := api.ShowSpace(c.Name)
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot retrieve space %q: %v", c.Name, err)
			}
			if params.IsCodeUnauthorized(err) {
				common.PermissionsMessage(ctx.Stderr, "retrieving space")
			}
			return block.ProcessBlockedError(errors.Annotatef(err, "cannot retrieve space %q", c.Name), block.BlockChange)
		}

		formatted := showSpaceFromResult(space)
		return errors.Trace(c.out.Write(ctx, formatted))
	})
}

// showSpaceFromResult converts params.ShowSpaceResult to ShowSpace
func showSpaceFromResult(result params.ShowSpaceResult) ShowSpace {
	s := result.Space
	subnets := make([]SubnetInfo, len(s.Subnets))
	for i, value := range s.Subnets {
		subnets[i].AvailabilityZones = value.Zones
		subnets[i].ProviderId = value.ProviderId
		subnets[i].VLANTag = value.VLANTag
		subnets[i].CIDR = value.CIDR
		subnets[i].ProviderNetworkId = value.ProviderNetworkId
		subnets[i].ProviderSpaceId = value.ProviderSpaceId
	}
	return ShowSpace{
		Space: SpaceInfo{
			ID:      s.Id,
			Name:    s.Name,
			Subnets: subnets,
		},
		Applications: result.Applications,
		MachineCount: result.MachineCount,
	}
}

// ShowSpace represents space information output by the CLI client.
type ShowSpace struct {
	// Information about a given space.
	Space SpaceInfo `json:"space" yaml:"space"`
	// Application names which are bound to a given space.
	Applications []string `json:"applications" yaml:"applications"`
	// MachineCount is the number of machines connected to a given space.
	MachineCount int `json:"machine-count" yaml:"machine-count"`
}
