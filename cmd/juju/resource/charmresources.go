// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/charmstore"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// CharmResourcesCommand implements the "juju charm-resources" command.
type CharmResourcesCommand struct {
	baseCharmResourcesCommand
}

// NewCharmResourcesCommand returns a new command that lists resources defined
// by a charm.
func NewCharmResourcesCommand(resourceLister ResourceLister) modelcmd.ModelCommand {
	var c CharmResourcesCommand
	c.setResourceLister(resourceLister)
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

// CharmResourceLister lists resources for the given charm ids.
type ResourceLister interface {
	ListResources(ids []charmstore.CharmID) ([][]charmresource.Resource, error)
}

type baseCharmResourcesCommand struct {
	modelcmd.ModelCommandBase

	// resourceLister is called by Run to list charm resources and
	// uses juju/juju/charmstore.Client.
	resourceLister ResourceLister

	out     cmd.Output
	channel string
	charm   string
}

func (b *baseCharmResourcesCommand) setResourceLister(resourceLister ResourceLister) {
	if resourceLister == nil {
		resourceLister = b
	}
	b.resourceLister = resourceLister
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
	// TODO(ericsnow) Adjust this to the charm store.

	charmURL, err := resolveCharm(c.charm)
	if err != nil {
		return errors.Trace(err)
	}
	charm := charmstore.CharmID{URL: charmURL, Channel: csparams.Channel(c.channel)}

	resources, err := c.resourceLister.ListResources([]charmstore.CharmID{charm})
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

var charmResourcesDoc = `
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

// ListCharmResources implements CharmResourceLister by getting the charmstore client
// from the command's ModelCommandBase.
func (c *baseCharmResourcesCommand) ListResources(ids []charmstore.CharmID) ([][]charmresource.Resource, error) {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conAPIRoot, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	csURL, err := getCharmStoreAPIURL(conAPIRoot)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := charmstore.NewCustomClientAtURL(bakeryClient, csURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.ListResources(ids)
}

func resolveCharm(raw string) (*charm.URL, error) {
	charmURL, err := charm.ParseURL(raw)
	if err != nil {
		return charmURL, errors.Trace(err)
	}

	if charmURL.Series == "bundle" {
		return charmURL, errors.Errorf("charm bundles are not supported")
	}

	return charmURL, nil
}

// getCharmStoreAPIURL consults the controller config for the charmstore api url to use.
var getCharmStoreAPIURL = func(conAPIRoot api.Connection) (string, error) {
	controllerAPI := controller.NewClient(conAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.CharmStoreURL(), nil
}
