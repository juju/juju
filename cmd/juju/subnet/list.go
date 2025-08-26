// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"net"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/life"
)

// NewListCommand returns a cammin used to list all subnets
// known to Juju.
func NewListCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&ListCommand{})
}

// ListCommand displays a list of all subnets known to Juju
type ListCommand struct {
	SubnetCommandBase

	SpaceName string
	ZoneName  string

	spaceTag *names.SpaceTag

	Out cmd.Output
}

const listCommandDoc = `
Displays a list of all subnets known to Juju. Results can be filtered
using the optional --space and/or --zone arguments to only display
subnets associated with a given network space and/or availability zone.

Like with other Juju commands, the output and its format can be changed
using the ` + "`--format`" + ` and ` + "`--output`" + ` (or ` + "`-o`" + `) optional arguments. Supported
output formats include ` + "`yaml`" + ` (default) and ` + "`json`" + `. To redirect the
output to a file, use ` + "`--output`" + `.
`

const listCommandExample = `
To list all subnets known to Juju:

    juju subnets

To list subnets associated with a specific network space:

    juju subnets --space my-space

To list subnets associated with a specific availability zone:

    juju subnets --zone my-zone
`

// Info is defined on the cmd.Command interface.
func (c *ListCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "subnets",
		Args:     "[--space <name>] [--zone <name>] [--format yaml|json] [--output <path>]",
		Purpose:  "List subnets known to Juju.",
		Doc:      strings.TrimSpace(listCommandDoc),
		Aliases:  []string{"list-subnets"},
		Examples: listCommandExample,
	})
}

// SetFlags is defined on the cmd.Command interface.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SubnetCommandBase.SetFlags(f)
	c.Out.AddFlags(f, "yaml", output.DefaultFormatters)

	f.StringVar(&c.SpaceName, "space", "", "Filter results by space name")
	f.StringVar(&c.ZoneName, "zone", "", "Filter results by zone name")
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *ListCommand) Init(args []string) error {
	// No arguments are accepted, just flags.
	err := cmd.CheckEmpty(args)
	if err != nil {
		return err
	}

	// Validate space name, if given and store as tag.
	c.spaceTag = nil
	if c.SpaceName != "" {
		tag, err := c.ValidateSpace(c.SpaceName)
		if err != nil {
			c.SpaceName = ""
			return err
		}
		c.spaceTag = &tag
	}
	return nil
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) error {
	return errors.Trace(c.RunWithAPI(ctx, func(api SubnetAPI, ctx *cmd.Context) error {
		// Validate space and/or zone, if given to display a nicer error
		// message.
		// Get the list of subnets, filtering them as requested.
		subnets, err := api.ListSubnets(c.spaceTag, c.ZoneName)
		if err != nil {
			return errors.Annotate(err, "cannot list subnets")
		}

		// Display a nicer message in case no subnets were found.
		if len(subnets) == 0 {
			if c.SpaceName != "" || c.ZoneName != "" {
				ctx.Infof("No subnets found matching requested criteria.")
			} else {
				ctx.Infof("No subnets to display.")
			}
			return nil
		}

		// Construct the output list for displaying with the chosen
		// format.
		result := formattedList{
			Subnets: make(map[string]formattedSubnet),
		}
		for _, sub := range subnets {
			subResult := formattedSubnet{
				ProviderId:        sub.ProviderId,
				ProviderNetworkId: sub.ProviderNetworkId,
				Zones:             sub.Zones,
			}

			// Use the CIDR to determine the subnet type.
			if ip, _, err := net.ParseCIDR(sub.CIDR); err != nil {
				return errors.Errorf("subnet %q has invalid CIDR", sub.CIDR)
			} else if ip.To4() != nil {
				subResult.Type = typeIPv4
			} else if ip.To16() != nil {
				subResult.Type = typeIPv6
			}
			if sub.SpaceTag != "" {
				// Space must be valid, but verify anyway.
				spaceTag, err := names.ParseSpaceTag(sub.SpaceTag)
				if err != nil {
					return errors.Annotatef(err, "subnet %q has invalid space", sub.CIDR)
				}
				subResult.Space = spaceTag.Id()
			}

			// Display correct status according to the life cycle value.
			switch sub.Life {
			case life.Alive:
				subResult.Status = statusInUse
			case life.Dying, life.Dead:
				subResult.Status = statusTerminating
			}

			result.Subnets[sub.CIDR] = subResult
		}

		return c.Out.Write(ctx, result)
	}))
}

const (
	typeIPv4 = "ipv4"
	typeIPv6 = "ipv6"

	statusInUse       = "in-use"
	statusTerminating = "terminating"
)

type formattedList struct {
	Subnets map[string]formattedSubnet `json:"subnets" yaml:"subnets"`
}

type formattedSubnet struct {
	Type              string   `json:"type" yaml:"type"`
	ProviderId        string   `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`
	ProviderNetworkId string   `json:"provider-network-id,omitempty" yaml:"provider-network-id,omitempty"`
	Status            string   `json:"status,omitempty" yaml:"status,omitempty"`
	Space             string   `json:"space" yaml:"space"`
	Zones             []string `json:"zones" yaml:"zones"`
}
