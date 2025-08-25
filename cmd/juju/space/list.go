// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"io"
	"net"
	"sort"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/life"
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
Displays all defined spaces.

By default both spaces and their subnets are displayed. Supplying the ` + "`--short`" + ` option will list just the space names.

The ` + "`--output`" + ` argument allows the command's output to be redirected to a file.
`

const listCommandExamples = `
List spaces and their subnets:

	juju spaces

List spaces:

	juju spaces --short
`

// Info is defined on the cmd.Command interface.
func (c *ListCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "spaces",
		Args:     "[--short] [--format yaml|json] [--output <path>]",
		Purpose:  "List known spaces, including associated subnets.",
		Doc:      strings.TrimSpace(listCommandDoc),
		Aliases:  []string{"list-spaces"},
		Examples: listCommandExamples,
		SeeAlso: []string{
			"add-space",
			"reload-spaces",
		},
	})
}

// SetFlags is defined on the cmd.Command interface.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.printTabular,
	})
	f.BoolVar(&c.Short, "short", false, "Only display spaces.")
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
	return c.RunWithSpaceAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
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
				result.Spaces = append(result.Spaces, spaceName(space.Name))
			}
			return c.out.Write(ctx, result)
		}

		// Construct the output list for displaying with the chosen
		// format.
		result := formattedList{
			Spaces: make([]formattedSpace, len(spaces)),
		}

		for i, space := range spaces {
			fsp := formattedSpace{
				Id:   space.Id,
				Name: space.Name,
			}

			result.Spaces[i].Id = space.Id

			fsn := make(map[string]formattedSubnet, len(space.Subnets))
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
				case life.Alive:
					subResult.Status = statusInUse
				case life.Dying, life.Dead:
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

				fsn[subnet.CIDR] = subResult
			}
			fsp.Subnets = fsn
			result.Spaces[i] = fsp
		}
		return c.out.Write(ctx, result)
	})
}

// printTabular prints the list of spaces in tabular format
func (c *ListCommand) printTabular(writer io.Writer, value interface{}) error {
	tw := output.TabWriter(writer)

	write := printTabularLong
	if c.Short {
		write = printTabularShort
	}
	if err := write(tw, value); err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(tw.Flush())
}

func printTabularShort(writer io.Writer, value interface{}) error {
	list, ok := value.(formattedShortList)
	if !ok {
		return errors.New("unexpected value")
	}

	_, _ = fmt.Fprintln(writer, "Space")
	spaces := list.Spaces
	sort.Strings(spaces)
	for _, space := range spaces {
		_, _ = fmt.Fprintf(writer, "%v\n", space)
	}

	return nil
}

func printTabularLong(writer io.Writer, value interface{}) error {
	list, ok := value.(formattedList)
	if !ok {
		return errors.New("unexpected value")
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	table.AddRow("Name", "Space ID", "Subnets")
	for _, s := range list.Spaces {
		if len(s.Subnets) == 0 {
			table.AddRow(spaceName(s.Name), s.Id, "")
			continue
		}

		var cidrs []string
		for cidr := range s.Subnets {
			cidrs = append(cidrs, cidr)
		}
		sort.Strings(cidrs)

		table.AddRow(spaceName(s.Name), s.Id, cidrs[0])
		for i := 1; i < len(cidrs); i++ {
			table.AddRow("", "", cidrs[i])
		}
	}

	table.AddRow("", "", "")
	_, _ = fmt.Fprintln(writer, table)
	return nil
}

const (
	typeUnknown = "unknown"
	typeIPv4    = "ipv4"
	typeIPv6    = "ipv6"

	statusInUse       = "in-use"
	statusTerminating = "terminating"
)

type formattedSubnet struct {
	Type       string   `json:"type" yaml:"type"`
	ProviderId string   `json:"provider-id,omitempty" yaml:"provider-id,omitempty"`
	Status     string   `json:"status,omitempty" yaml:"status,omitempty"`
	Zones      []string `json:"zones" yaml:"zones"`
}

type formattedSpace struct {
	Id      string                     `json:"id" yaml:"id"`
	Name    string                     `json:"name" yaml:"name"`
	Subnets map[string]formattedSubnet `json:"subnets" yaml:"subnets"`
}

type formattedList struct {
	Spaces []formattedSpace `json:"spaces" yaml:"spaces"`
}

type formattedShortList struct {
	Spaces []string `json:"spaces" yaml:"spaces"`
}

func spaceName(name string) string {
	return name
}
