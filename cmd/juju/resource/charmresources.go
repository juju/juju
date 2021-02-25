// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/charmstore"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
)

// ResourceLister lists resources for the given charm ids.
type ResourceLister interface {
	ListResources(ids []CharmID) ([][]charmresource.Resource, error)
}

// CharmID represents the charm identifier.
type CharmID struct {
	// URL is the url of the charm.
	URL *charm.URL

	// Channel is the channel in which the charm was published.
	Channel corecharm.Channel
}

// ResourceListerDependencies defines the dependencies to create a store
// dependant resource listener.
type ResourceListerDependencies interface {
	BakeryClient() (*httpbakery.Client, error)
	NewControllerAPIRoot() (api.Connection, error)
}

// CreateResourceListener defines a factory function to create a resource
// listener.
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
		Args:    "<charm>",
		Purpose: "Display the resources for a charm in the charm store.",
		Doc:     charmResourcesDoc,
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
	f.StringVar(&c.channel, "channel", "stable", "the charmstore channel of the charm")
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

	var channel corecharm.Channel
	if charm.CharmHub.Matches(charmURL.Schema) {
		channel, err = corecharm.ParseChannelNormalize(c.channel)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		channel = corecharm.MakePermissiveChannel("", c.channel, "")
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
This command will report the resources for a charm in the charm store.

<charm> can be a charm URL, or an unambiguously condensed form of it,
just like the deploy command. So the following forms will be accepted:

For cs:trusty/mysql
  mysql
  trusty/mysql

For cs:~user/trusty/mysql
  cs:~user/mysql

Where the series is not supplied, the series from your local host is used.
Thus the above examples imply that the local series is trusty.
`

func resolveCharm(raw string) (*charm.URL, error) {
	charmURL, err := charm.ParseURL(raw)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if charmURL.Series == "bundle" {
		return nil, errors.NotSupportedf("charm bundles")
	}

	return charmURL, nil
}

func defaultResourceLister(schema string, deps ResourceListerDependencies) (ResourceLister, error) {
	if charm.CharmHub.Matches(schema) {
		return nil, errors.Errorf("charmhub charms are currently not supported")
	}

	return &CharmStoreResourceListener{
		BakeryClientFn:      deps.BakeryClient,
		ControllerAPIRootFn: deps.NewControllerAPIRoot,
	}, nil
}

// BakeryClient defines a way to create a bakery client.
type BakeryClient = func() (*httpbakery.Client, error)

// ControllerAPIRoot defines a way to create a new controller API root.
type ControllerAPIRoot = func() (api.Connection, error)

// CharmStoreResourceListener defines a charm store resource listener.
type CharmStoreResourceListener struct {
	BakeryClientFn      BakeryClient
	ControllerAPIRootFn ControllerAPIRoot
}

// ListResources implements CharmResourceLister.
func (c *CharmStoreResourceListener) ListResources(ids []CharmID) ([][]charmresource.Resource, error) {
	bakeryClient, err := c.BakeryClientFn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conAPIRoot, err := c.ControllerAPIRootFn()
	if err != nil {
		return nil, errors.Trace(err)
	}
	csURL, err := c.getCharmStoreAPIURL(conAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := charmstore.NewCustomClientAtURL(bakeryClient, csURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmIDs := make([]charmstore.CharmID, len(ids))
	for i, id := range ids {
		charmIDs[i] = charmstore.CharmID{
			URL:     id.URL,
			Channel: csparams.Channel(id.Channel.Risk),
		}
	}

	return client.ListResources(charmIDs)
}

// getCharmStoreAPIURL consults the controller config for the charmstore api url to use.
func (c *CharmStoreResourceListener) getCharmStoreAPIURL(conAPIRoot api.Connection) (string, error) {
	controllerAPI := controller.NewClient(conAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.CharmStoreURL(), nil
}
