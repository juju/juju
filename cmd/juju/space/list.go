// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"io"
	"net"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// NewListCommand returns a command used to list spaces.
func NewListCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&ListCommand{})
}

// listCommand displays a list of all spaces known to Juju.
type ListCommand struct {
	SpaceCommandBase
	Short bool
	out   cmd.Output
}

const listCommandDoc = `
Displays all defined spaces. If --short is not given both spaces and
their subnets are displayed, otherwise just a list of spaces. The
--format argument has the same semantics as in other CLI commands -
"yaml" is the default. The --output argument allows the command
output to be redirected to a file. `

// Info is defined on the cmd.Command interface.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "spaces",
		Args:    "[--short] [--format yaml|json] [--output <path>]",
		Purpose: "List known spaces, including associated subnets.",
		Doc:     strings.TrimSpace(listCommandDoc),
		Aliases: []string{"list-spaces"},
	}
}

// SetFlags is defined on the cmd.Command interface.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.printTabular,
	})
	f.BoolVar(&c.Short, "short", false, "only display spaces.")
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *ListCommand) Init(args []string) error {
	// No arguments are accepted, just flags.
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) error {
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		spaces, err := api.ListSpaces()
		if err != nil {
			if errors.IsNotSupported(err) {
				ctx.Infof("cannot list spaces: %v", err)
			}
			return errors.Annotate(err, "cannot list spaces")
		}
		if len(spaces) == 0 {
			ctx.Infof("no spaces to display")
			if c.out.Name() == "tabular" {
				return nil
			}
		}

		if c.Short {
			result := formattedShortList{}
			for _, space := range spaces {
				result.Spaces = append(result.Spaces, space.Name)
			}
			return c.out.Write(ctx, result)
		}
		// Construct the output list for displaying with the chosen
		// format.
		result := formattedList{
			Spaces: make(map[string]map[string]formattedSubnet),
		}

		for _, space := range spaces {
			result.Spaces[space.Name] = make(map[string]formattedSubnet)
			for _, subnet := range space.Subnets {
				subResult := formattedSubnet{
					Type:       typeUnknown,
					ProviderId: subnet.ProviderId,
					Zones:      subnet.Zones,
				}
				// Display correct status according to the life cycle value.
				//
				// TODO(dimitern): Do this on the apiserver side, also
				// do the same for params.Space, so in case of an
				// error it can be displayed.
				switch subnet.Life {
				case params.Alive:
					subResult.Status = statusInUse
				case params.Dying, params.Dead:
					subResult.Status = statusTerminating
				}

				// Use the CIDR to determine the subnet type.
				// TODO(dimitern): Do this on the apiserver side.
				if ip, _, err := net.ParseCIDR(subnet.CIDR); err != nil {
					// This should never happen as subnets will be
					// validated before saving in state.
					msg := fmt.Sprintf("error: invalid subnet CIDR: %s", subnet.CIDR)
					subResult.Status = msg
				} else if ip.To4() != nil {
					subResult.Type = typeIPv4
				} else if ip.To16() != nil {
					subResult.Type = typeIPv6
				}
				result.Spaces[space.Name][subnet.CIDR] = subResult
			}
		}
		return c.out.Write(ctx, result)
	})
}

// printTabular prints the list of spaces in tabular format
func (c *ListCommand) printTabular(writer io.Writer, value interface{}) error {
	tw := output.TabWriter(writer)
	if c.Short {
		list, ok := value.(formattedShortList)
		if !ok {
			return errors.New("unexpected value")
		}
		fmt.Fprintln(tw, "Space")
		spaces := list.Spaces
		sort.Strings(spaces)
		for _, space := range spaces {
			fmt.Fprintf(tw, "%v\n", space)
		}
	} else {
		list, ok := value.(formattedList)
		if !ok {
			return errors.New("unexpected value")
		}

		fmt.Fprintf(tw, "%s\t%s\n", "Space", "Subnets")
		spaces := []string{}
		for name, _ := range list.Spaces {
			spaces = append(spaces, name)
		}
		sort.Strings(spaces)
		for _, name := range spaces {
			subnets := list.Spaces[name]
			fmt.Fprintf(tw, "%s", name)
			if len(subnets) == 0 {
				fmt.Fprintf(tw, "\n")
				continue
			}
			cidrs := []string{}
			for subnet, _ := range subnets {
				cidrs = append(cidrs, subnet)
			}
			sort.Strings(cidrs)
			for _, cidr := range cidrs {
				fmt.Fprintf(tw, "\t%v\n", cidr)
			}
		}
	}
	tw.Flush()
	return nil
}

const (
	typeUnknown = "unknown"
	typeIPv4    = "ipv4"
	typeIPv6    = "ipv6"

	statusInUse       = "in-use"
	statusTerminating = "terminating"
)

// TODO(dimitern): Display space attributes along with subnets (state
// or error,public,?)

type formattedList struct {
	Spaces map[string]map[string]formattedSubnet `json:"spaces" yaml:"spaces"`
}

type formattedShortList struct {
	Spaces []string `json:"spaces" yaml:"spaces"`
}

type formattedSubnet struct {
	Type       string   `json:"type" yaml:"type"`
	ProviderId string   `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`
	Status     string   `json:"status,omitempty" yaml:"status,omitempty"`
	Zones      []string `json:"zones" yaml:"zones"`
}
