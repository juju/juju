// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

// CharmCommandBase exposes the functionality of charmcmd.CommandBase
// needed here.
type CharmCommandBase interface {
	// Connect connects to the charm store and returns a client.
	Connect() (CharmResourceLister, error)
}

// CharmResourceLister has the charm store API methods needed by ListCharmResourcesCommand.
type CharmResourceLister interface {
	// ListResources lists the resources for each of the identified charms.
	ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error)

	// Close closes the client.
	Close() error
}

// ListCharmResourcesCommand implements the "juju charm list-resources" command.
type ListCharmResourcesCommand struct {
	modelcmd.ModelCommandBase
	CharmCommandBase
	out   cmd.Output
	charm string
}

// NewListCharmResourcesCommand returns a new command that lists resources defined
// by a charm.
func NewListCharmResourcesCommand(base CharmCommandBase) *ListCharmResourcesCommand {
	cmd := &ListCharmResourcesCommand{
		CharmCommandBase: base,
	}
	return cmd
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
		Name:    "list-resources",
		Args:    "<charm>",
		Purpose: "display the resources for a charm in the charm store",
		Doc:     listCharmResourcesDoc,
		Aliases: []string{"resources"},
	}
}

// SetFlags implements cmd.Command.
func (c *ListCharmResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatCharmTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
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

	apiclient, err := c.Connect()
	if err != nil {
		// TODO(ericsnow) Return a more user-friendly error?
		return errors.Trace(err)
	}
	defer apiclient.Close()

	charmURLs, err := resolveCharms([]string{c.charm})
	if err != nil {
		return errors.Trace(err)
	}

	resources, err := apiclient.ListResources(charmURLs)
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
