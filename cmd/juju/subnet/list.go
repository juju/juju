// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"encoding/json"
	"net"
	"strings"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
)

// ListCommand displays a list of all subnets known to Juju
type ListCommand struct {
	SubnetCommandBase

	SpaceName string
	ZoneName  string

	spaceTag *names.SpaceTag

	out cmd.Output
}

const listCommandDoc = `
Displays a list of all subnets known to Juju. Results can be filtered
using the optional --space and/or --zone arguments to only display
subnets associated with a given network space and/or availability zone.

Like with other Juju commands, the output and its format can be changed
using the --format and --output (or -o) optional arguments. Supported
output formats include "yaml" (default) and "json". To redirect the
output to a file, use --output.
`

// Info is defined on the cmd.Command interface.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Args:    "[--space <name>] [--zone <name>] [--format yaml|json] [--output <path>]",
		Purpose: "list subnets known to Juju",
		Doc:     strings.TrimSpace(listCommandDoc),
	}
}

// SetFlags is defined on the cmd.Command interface.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SubnetCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})

	f.StringVar(&c.SpaceName, "space", "", "filter results by space name")
	f.StringVar(&c.ZoneName, "zone", "", "filter results by zone name")
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
	return c.RunWithAPI(ctx, func(api SubnetAPI, ctx *cmd.Context) error {
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
				ctx.Infof("no subnets found matching requested criteria")
			} else {
				ctx.Infof("no subnets to display")
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
				ProviderId: sub.ProviderId,
				Zones:      sub.Zones,
			}

			// Use the CIDR to determine the subnet type.
			if ip, _, err := net.ParseCIDR(sub.CIDR); err != nil {
				return errors.Errorf("subnet %q has invalid CIDR", sub.CIDR)
			} else if ip.To4() != nil {
				subResult.Type = typeIPv4
			} else if ip.To16() != nil {
				subResult.Type = typeIPv6
			}
			// Space must be valid, but verify anyway.
			spaceTag, err := names.ParseSpaceTag(sub.SpaceTag)
			if err != nil {
				return errors.Annotatef(err, "subnet %q has invalid space", sub.CIDR)
			}
			subResult.Space = spaceTag.Id()

			// Display correct status according to the life cycle value.
			switch sub.Life {
			case params.Alive:
				subResult.Status = statusInUse
			case params.Dying, params.Dead:
				subResult.Status = statusTerminating
			}

			result.Subnets[sub.CIDR] = subResult
		}

		return c.out.Write(ctx, result)
	})
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

// A goyaml bug means we can't declare these types locally to the
// GetYAML methods.
type formattedListNoMethods formattedList

// MarshalJSON is defined on json.Marshaller.
func (l formattedList) MarshalJSON() ([]byte, error) {
	return json.Marshal(formattedListNoMethods(l))
}

// GetYAML is defined on yaml.Getter.
func (l formattedList) GetYAML() (tag string, value interface{}) {
	return "", formattedListNoMethods(l)
}

type formattedSubnet struct {
	Type       string   `json:"type" yaml:"type"`
	ProviderId string   `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`
	Status     string   `json:"status,omitempty" yaml:"status,omitempty"`
	Space      string   `json:"space" yaml:"space"`
	Zones      []string `json:"zones" yaml:"zones"`
}

// A goyaml bug means we can't declare these types locally to the
// GetYAML methods.
type formattedSubnetNoMethods formattedSubnet

// MarshalJSON is defined on json.Marshaller.
func (s formattedSubnet) MarshalJSON() ([]byte, error) {
	return json.Marshal(formattedSubnetNoMethods(s))
}

// GetYAML is defined on yaml.Getter.
func (s formattedSubnet) GetYAML() (tag string, value interface{}) {
	return "", formattedSubnetNoMethods(s)
}
