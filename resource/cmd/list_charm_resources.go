// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
)

// CharmResourceLister lists resources for the given charm ids.
type ResourceLister interface {
	ListResources(ids []charmstore.CharmID) ([][]charmresource.Resource, error)
}

// ListCharmResourcesCommand implements the "juju charm resources" command.
type ListCharmResourcesCommand struct {
	modelcmd.ModelCommandBase

	// ResourceLister is called by Run to list charm resources. The
	// default implementation uses juju/juju/charmstore.Client, but
	// it may be set to mock out the call to that method.
	ResourceLister ResourceLister

	out     cmd.Output
	channel string
	charm   string
}

// NewListCharmResourcesCommand returns a new command that lists resources defined
// by a charm.
func NewListCharmResourcesCommand() *ListCharmResourcesCommand {
	var c ListCharmResourcesCommand
	c.ResourceLister = &c
	return &c
}

var listCharmResourcesDoc = `
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

// Info implements cmd.Command.
func (c *ListCharmResourcesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "resources",
		Args:    "<charm>",
		Purpose: "display the resources for a charm in the charm store",
		Doc:     listCharmResourcesDoc,
		Aliases: []string{"list-resources"},
	}
}

// SetFlags implements cmd.Command.
func (c *ListCharmResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatCharmTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
	f.StringVar(&c.channel, "channel", "stable", "the charmstore channel of the charm")
}

// Init implements cmd.Command.
func (c *ListCharmResourcesCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing charm")
	}
	c.charm = args[0]

	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements cmd.Command.
func (c *ListCharmResourcesCommand) Run(ctx *cmd.Context) error {
	// TODO(ericsnow) Adjust this to the charm store.

	charmURLs, err := resolveCharms([]string{c.charm})
	if err != nil {
		return errors.Trace(err)
	}

	charms := make([]charmstore.CharmID, len(charmURLs))
	for i, id := range charmURLs {
		charms[i] = charmstore.CharmID{URL: id, Channel: csparams.Channel(c.channel)}
	}

	resources, err := c.ResourceLister.ListResources(charms)
	if err != nil {
		return errors.Trace(err)
	}
	if len(resources) != 1 {
		return errors.New("got bad data from charm store")
	}

	// Note that we do not worry about c.CompatVersion
	// for show-charm-resources...
	formatter := newCharmResourcesFormatter(resources[0])
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}

// ListCharmResources implements CharmResourceLister by getting the charmstore client
// from the command's ModelCommandBase.
func (c *ListCharmResourcesCommand) ListResources(ids []charmstore.CharmID) ([][]charmresource.Resource, error) {
	apiContext, err := c.APIContext()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We use the default for URL.
	client, err := charmstore.NewCustomClient(apiContext.BakeryClient, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.ListResources(ids)
}

func resolveCharms(charms []string) ([]*charm.URL, error) {
	var charmURLs []*charm.URL
	for _, raw := range charms {
		charmURL, err := resolveCharm(raw)
		if err != nil {
			return nil, errors.Trace(err)
		}
		charmURLs = append(charmURLs, charmURL)
	}
	return charmURLs, nil
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
