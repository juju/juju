// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"encoding/json"
	"net"
	"sort"
	"strings"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/subnet"
	"github.com/juju/juju/network"
)

// ListCommand displays a list of all spaces known to Juju.
type ListCommand struct {
	SpaceCommandBase
	Short     bool
	out       cmd.Output
	subnetapi subnet.SubnetAPI
}

const listCommandDoc = `
Displays all defined spaces. If --short is not given both spaces and their subnets  are displayed, otherwise just a list
of spaces. The --format argument has the same semantics as in other CLI commands - "yaml" is the default. The --output
argument always exists when --format is supported, and allows to redirect the command output to a file.
`

// Info is defined on the cmd.Command interface.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Args:    "[--short] [--format yaml|json] [--output <path>]",
		Purpose: "list spaces known to Juju, including associated subnets",
		Doc:     strings.TrimSpace(listCommandDoc),
	}
}

// SetFlags is defined on the cmd.Command interface.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})

	f.BoolVar(&c.Short, "short", false, "only display spaces.")
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *ListCommand) Init(args []string) error {
	// No arguments are accepted, just flags.
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}

	return nil
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewAPI()
	subnetapi, err := c.NewSubnetAPI()
	if err != nil {
		return errors.Annotate(err, "cannot connect to API server")
	}
	defer api.Close()
	defer subnetapi.Close()

	spaces, err := api.AllSpaces()
	if err != nil {
		return errors.Annotate(err, "cannot list spaces")
	}
	if len(spaces) == 0 {
		ctx.Infof("no spaces to display")
		return c.out.Write(ctx, nil)
	}

	// Sort spaces by name
	sort.Sort(network.BySpaceName(spaces))

	// Get the list of subnets
	subnets, err := subnetapi.ListSubnets(nil, "")
	if err != nil {
		return errors.Annotate(err, "cannot list subnets")
	}
	if len(subnets) == 0 {
		return errors.NotValidf("no subnets found, but found spaces:")
	}

	// Create a map of CIDR -> subnet
	subnetMap := make(map[string]params.Subnet)
	for _, subnet := range subnets {
		subnetMap[subnet.CIDR] = subnet
	}

	// Construct the output list for displaying with the chosen
	// format.
	result := formattedList{
		Spaces: make(map[string]map[string]formattedSubnet),
	}

	for _, space := range spaces {
		result.Spaces[space.Name] = make(map[string]formattedSubnet)
		for _, CIDR := range space.CIDRs {
			sub := subnetMap[CIDR]
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

			// Display correct status according to the life cycle value.
			switch sub.Life {
			case params.Alive:
				subResult.Status = statusInUse
			case params.Dying, params.Dead:
				subResult.Status = statusTerminating
			}
			result.Spaces[space.Name][CIDR] = subResult
		}
	}

	return c.out.Write(ctx, result)
}

const (
	typeIPv4 = "ipv4"
	typeIPv6 = "ipv6"

	statusInUse       = "in-use"
	statusTerminating = "terminating"
)

type formattedList struct {
	Spaces map[string]map[string]formattedSubnet `json:"spaces" yaml:"spaces"`
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

func (c ListCommand) NewSubnetAPI() (subnet.SubnetAPI, error) {
	// TODO(dimitern): Change this once the API is implemented.

	if c.subnetapi != nil {
		// Already created.
		return c.subnetapi, nil
	}

	return nil, errors.New("API not implemented yet!")
}
