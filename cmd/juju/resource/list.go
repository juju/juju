// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
)

// ListClient has the API client methods needed by ListCommand.
type ListClient interface {
	// ListResources returns info about resources for applications in the model.
	ListResources(applications []string) ([]resource.ApplicationResources, error)
	// Close closes the connection.
	Close() error
}

// ListDeps is a type that contains external functions that List needs.
type ListDeps struct {
	// NewClient returns the value that wraps the API for showing
	// resources from the server.
	NewClient func(*ListCommand) (ListClient, error)
}

// ListCommand discovers and lists application or unit resources.
type ListCommand struct {
	modelcmd.ModelCommandBase

	details bool
	deps    ListDeps
	out     cmd.Output
	target  string
}

// NewListCommand returns a new command that lists resources defined
// by a charm.
func NewListCommand(deps ListDeps) modelcmd.ModelCommand {
	return modelcmd.Wrap(&ListCommand{deps: deps})
}

// Info implements cmd.Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "resources",
		Aliases: []string{"list-resources"},
		Args:    "<application or unit>",
		Purpose: "Show the resources for an application or unit.",
		Doc: `
This command shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from the charmstore.
`,
	})
}

// SetFlags implements cmd.Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	const defaultFormat = "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		defaultFormat: FormatAppTabular,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})

	f.BoolVar(&c.details, "details", false, "show detailed information about resources used by each unit.")
}

// Init implements cmd.Command.Init. It will return an error satisfying
// errors.BadRequest if you give it an incorrect number of arguments.
func (c *ListCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.NewBadRequest(nil, "missing application or unit name")
	}
	c.target = args[0]
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.NewBadRequest(err, "")
	}
	return nil
}

// Run implements cmd.Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.deps.NewClient(c)
	if err != nil {
		return errors.Trace(err)
	}
	defer apiclient.Close()

	var unit string
	var application string
	if names.IsValidApplication(c.target) {
		application = c.target
	} else {
		application, err = names.UnitApplication(c.target)
		if err != nil {
			return errors.Errorf("%q is neither an application nor a unit", c.target)
		}
		unit = c.target
	}

	vals, err := apiclient.ListResources([]string{application})
	if err != nil {
		return errors.Trace(err)
	}
	if len(vals) != 1 {
		return errors.Errorf("bad data returned from server")
	}
	v := vals[0]

	// It's a lot easier to read and to digest a list of resources
	// when  they are ordered.
	sort.Sort(charmResourceList(v.CharmStoreResources))
	sort.Sort(resourceList(v.Resources))
	for _, u := range v.UnitResources {
		sort.Sort(resourceList(u.Resources))
	}

	if unit == "" {
		return c.formatApplicationResources(ctx, v)
	}
	return c.formatUnitResources(ctx, unit, application, v)
}

const noResources = "No resources to display."

func (c *ListCommand) formatApplicationResources(ctx *cmd.Context, sr resource.ApplicationResources) error {
	if c.details {
		formatted, err := FormatApplicationDetails(sr)
		if err != nil {
			return errors.Trace(err)
		}
		if len(formatted.Resources) == 0 && len(formatted.Updates) == 0 {
			ctx.Infof(noResources)
			return nil
		}

		return c.out.Write(ctx, formatted)
	}

	formatted, err := formatApplicationResources(sr)
	if err != nil {
		return errors.Trace(err)
	}
	if len(formatted.Resources) == 0 && len(formatted.Updates) == 0 {
		ctx.Infof(noResources)
		return nil
	}
	return c.out.Write(ctx, formatted)
}

func (c *ListCommand) formatUnitResources(ctx *cmd.Context, unit, application string, sr resource.ApplicationResources) error {
	if len(sr.Resources) == 0 && len(sr.UnitResources) == 0 {
		ctx.Infof(noResources)
		return nil
	}

	if c.details {
		formatted := detailedResources(unit, sr)
		return c.out.Write(ctx, FormattedUnitDetails(formatted))
	}

	resources := unitResources(unit, application, sr)
	res := make([]FormattedAppResource, len(sr.Resources))
	for i, r := range sr.Resources {
		if unitResource, ok := resources[r.ID]; ok {
			// Unit has this application resource,
			// so use unit's version.
			r = unitResource
		} else {
			// Unit does not have this application resource.
			// Have to set it to -1 since revision 0 is still a valid revision.
			// All other information is inherited from application resource.
			r.Revision = -1
		}
		res[i] = FormatAppResource(r)
	}

	return c.out.Write(ctx, res)

}

func unitResources(unit, application string, sr resource.ApplicationResources) map[string]resource.Resource {
	var resources []resource.Resource
	for _, res := range sr.UnitResources {
		if res.Tag.Id() == unit {
			resources = res.Resources
		}
	}
	if len(resources) == 0 {
		return nil
	}
	unitResourcesById := make(map[string]resource.Resource)
	for _, r := range resources {
		unitResourcesById[r.ID] = r
	}
	return unitResourcesById
}
