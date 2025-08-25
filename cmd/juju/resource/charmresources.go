// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// ResourceLister lists resources for the given charm ids.
type ResourceLister interface {
	ListResources(ids []CharmID) ([][]charmresource.Resource, error)
}

// CharmResourceLister lists the resource of a charm.
type CharmResourceLister interface {
	ListCharmResources(curl string, origin apicharm.Origin) ([]charmresource.Resource, error)
}

// CharmID represents the charm identifier.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Channel is the channel in which the charm was published.
	Channel charm.Channel
}

// APIRoot defines a way to create a new API root.
type APIRoot = func() (api.Connection, error)

// ResourceListerDependencies defines the dependencies to create a store
// dependant resource lister.
type ResourceListerDependencies interface {
	NewAPIRoot() (api.Connection, error)
}

// CreateResourceListener defines a factory function to create a resource
// lister.
type CreateResourceListener = func(string, ResourceListerDependencies) (ResourceLister, error)

// CharmResourcesCommand implements the "juju charm-resources" command.
type CharmResourcesCommand struct {
	baseCharmResourcesCommand
}

// NewCharmResourcesCommand returns a new command that lists resources defined
// by a charm.
func NewCharmResourcesCommand() modelcmd.ModelCommand {
	c := CharmResourcesCommand{
		baseCharmResourcesCommand{
			CreateResourceListerFn: defaultResourceLister,
		},
	}
	return modelcmd.Wrap(&c)
}

// NewCharmResourcesCommandWithClient returns a new command that lists resources
// defined by a charm.
func NewCharmResourcesCommandWithClient(client ResourceLister) modelcmd.ModelCommand {
	c := CharmResourcesCommand{
		baseCharmResourcesCommand{
			CreateResourceListerFn: func(schema string, deps ResourceListerDependencies) (ResourceLister, error) {
				return client, nil
			},
		},
	}
	return modelcmd.Wrap(&c)
}

// Info implements cmd.Command.
func (c *CharmResourcesCommand) Info() *cmd.Info {
	i := c.baseInfo()
	i.Name = "charm-resources"
	i.Aliases = []string{"list-charm-resources"}
	return jujucmd.Info(i)
}

// SetFlags implements cmd.Command.
func (c *CharmResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.setBaseFlags(f)
}

// Init implements cmd.Command.
func (c *CharmResourcesCommand) Init(args []string) error {
	return c.baseInit(args)
}

// Run implements cmd.Command.
func (c *CharmResourcesCommand) Run(ctx *cmd.Context) error {
	return c.baseRun(ctx)
}

type baseCharmResourcesCommand struct {
	modelcmd.ModelCommandBase

	CreateResourceListerFn CreateResourceListener

	out     cmd.Output
	channel string
	charm   string
}

func (c *baseCharmResourcesCommand) baseInfo() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Args:     "<charm>",
		Purpose:  "Display the resources for a charm in a repository.",
		Doc:      charmResourcesDoc,
		Examples: charmResourcesExamples,
		SeeAlso: []string{
			"resources",
			"attach-resource",
		},
	})
}

func (c *baseCharmResourcesCommand) setBaseFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatCharmTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
	f.StringVar(&c.channel, "channel", "stable", "the channel of the charm")
}

func (c *baseCharmResourcesCommand) baseInit(args []string) error {
	if len(args) == 0 {
		return errors.New("missing charm")
	}
	c.charm = args[0]

	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *baseCharmResourcesCommand) baseRun(ctx *cmd.Context) error {
	charmURL, err := resolveCharm(c.charm)
	if errors.IsNotSupported(err) {
		if c.out.Name() == "tabular" {
			ctx.Infof("Bundles have no resources to display.")
			return nil
		}
		return c.out.Write(ctx, struct{}{})
	}
	if err != nil {
		return errors.Trace(err)
	}

	var channel charm.Channel
	if charm.CharmHub.Matches(charmURL.Schema) {
		channel, err = charm.ParseChannelNormalize(c.channel)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		channel = charm.MakePermissiveChannel("", c.channel, "")
	}

	resourceLister, err := c.CreateResourceListerFn(charmURL.Schema, c)
	if err != nil {
		return errors.Trace(err)
	}

	charm := CharmID{
		URL:     charmURL,
		Channel: channel,
	}

	resources, err := resourceLister.ListResources([]CharmID{
		charm,
	})
	if err != nil {
		return errors.Trace(err)
	}
	if len(resources) != 1 {
		return errors.New("got bad data from charm store")
	}
	res := resources[0]

	if len(res) == 0 && c.out.Name() == "tabular" {
		ctx.Infof("No resources to display.")
		return nil
	}

	// Note that we do not worry about c.CompatVersion
	// for show-charm-resources...
	formatter := newCharmResourcesFormatter(resources[0])
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}

const charmResourcesDoc = `
This command will report the resources and the current revision of each
resource for a charm in a repository.

Channel can be specified with ` + "`--channel`" + `.  If not provided, ` + "`stable`" + ` is used.

`

const charmResourcesExamples = `
Display charm resources for the ` + "`postgresql`" + ` charm:

    juju charm-resources postgresql

Display charm resources for ` + "`mycharm`" + ` in the ` + "`2.0/edge`" + ` channel:

    juju charm-resources mycharm --channel 2.0/edge

`

func resolveCharm(raw string) (*charm.URL, error) {
	charmURL, err := charm.ParseURL(raw)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !charm.CharmHub.Matches(charmURL.Schema) {
		return nil, errors.BadRequestf("only supported with charmhub charms")
	}

	return charmURL, nil
}

func defaultResourceLister(schema string, deps ResourceListerDependencies) (ResourceLister, error) {
	return &CharmhubResourceLister{
		APIRootFn: deps.NewAPIRoot,
	}, nil
}

// CharmhubResourceLister defines a charm hub resource lister.
type CharmhubResourceLister struct {
	APIRootFn APIRoot
}

// ListResources implements CharmResourceLister.
func (c *CharmhubResourceLister) ListResources(ids []CharmID) ([][]charmresource.Resource, error) {
	if len(ids) != 1 {
		return nil, errors.Errorf("expected one resource to list")
	}
	id := ids[0]
	var track *string
	if id.Channel.Track != "" {
		track = &id.Channel.Track
	}

	apiRoot, err := c.APIRootFn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := charms.NewClient(apiRoot)
	results, err := client.ListCharmResources(id.URL.String(), apicharm.Origin{
		Source: apicharm.OriginCharmHub,
		Track:  track,
		Risk:   string(id.Channel.Risk),
	})
	if errors.Is(err, errors.NotSupported) {
		return nil, errors.Errorf("charmhub charms are not supported with the current controller, try upgrading the controller to a newer version")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return [][]charmresource.Resource{results}, nil
}
