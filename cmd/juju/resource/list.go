// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"sort"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/resources"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	coreresources "github.com/juju/juju/core/resources"
)

// ListClient has the API client methods needed by ListCommand.
type ListClient interface {
	// ListResources returns info about resources for applications in the model.
	ListResources(applications []string) ([]coreresources.ApplicationResources, error)
	// Close closes the connection.
	Close() error
}

// ListCommand discovers and lists application or unit resources.
type ListCommand struct {
	modelcmd.ModelCommandBase

	newClient func() (ListClient, error)

	details bool
	out     cmd.Output
	target  string
}

// NewListCommand returns a new command that lists resources defined
// by a charm.
func NewListCommand() modelcmd.ModelCommand {
	c := &ListCommand{}
	c.newClient = func() (ListClient, error) {
		apiRoot, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return resources.NewClient(apiRoot)
	}
	return modelcmd.Wrap(c)
}

const listResourcesExamples = `
To list resources for an application:

	juju resources mysql

To list resources for a unit:

	juju resources mysql/0

To show detailed information about resources used by a unit:

	juju resources mysql/0 --details
`

// Info implements cmd.Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "resources",
		Aliases: []string{"list-resources"},
		Args:    "<application or unit>",
		Purpose: "Show the resources for an application or unit.",
		SeeAlso: []string{
			"attach-resource",
			"charm-resources",
		},
		Doc: `
This command shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from a store.
`,
		Examples: listResourcesExamples,
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

	f.BoolVar(&c.details, "details", false, "Show detailed information about the resources used by each unit.")
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
	apiclient, err := c.newClient()
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
	// when they are ordered.
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

func (c *ListCommand) formatApplicationResources(ctx *cmd.Context, sr coreresources.ApplicationResources) error {
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

func (c *ListCommand) formatUnitResources(ctx *cmd.Context, unit, application string, sr coreresources.ApplicationResources) error {
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

func unitResources(unit, application string, sr coreresources.ApplicationResources) map[string]coreresources.Resource {
	var res []coreresources.Resource
	for _, r := range sr.UnitResources {
		if r.Tag.Id() == unit {
			res = r.Resources
		}
	}
	if len(res) == 0 {
		return nil
	}
	unitResourcesById := make(map[string]coreresources.Resource)
	for _, r := range res {
		unitResourcesById[r.ID] = r
	}
	return unitResourcesById
}
