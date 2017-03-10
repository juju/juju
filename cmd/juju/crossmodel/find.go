// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

const findCommandDoc = `
Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (tabular|json|yaml)

Examples:
   $ juju find-endpoints local:
   $ juju find-endpoints local:/u/fred
   $ juju find-endpoints --interface mysql --url local:
   $ juju find-endpoints --charm db2 --url vendor:/u/ibm
   $ juju find-endpoints --charm db2 --author ibm
`

type findCommand struct {
	CrossModelCommandBase

	url           string
	interfaceName string
	endpoint      string
	user          string
	charm         string
	author        string

	out        cmd.Output
	newAPIFunc func() (FindAPI, error)
}

// NewFindEndpointsCommand constructs command that
// allows to find offered application endpoints.
func NewFindEndpointsCommand() cmd.Command {
	findCmd := &findCommand{}
	findCmd.newAPIFunc = func() (FindAPI, error) {
		return findCmd.NewCrossModelAPI()
	}
	return modelcmd.Wrap(findCmd)
}

// Init implements Command.Init.
func (c *findCommand) Init(args []string) (err error) {
	url, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}

	if url != "" {
		if c.url != "" {
			return errors.New("URL term cannot be specified twice")
		}
		c.url = url
	}
	if c.url != "" {
		if _, err := crossmodel.ParseApplicationURLParts(c.url); err != nil {
			return err
		}
	} else {
		// We need at least one filter. The default filter will list all local services.
		c.url = "local:"
	}
	return nil
}

// Info implements Command.Info.
func (c *findCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "find-endpoints",
		Purpose: "Find offered application endpoints",
		Doc:     findCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	f.StringVar(&c.url, "url", "", "application URL")
	f.StringVar(&c.interfaceName, "interface", "", "return results matching the interface name")
	f.StringVar(&c.endpoint, "endpoint", "", "return results matching the endpoint name")
	f.StringVar(&c.user, "user", "", "return results with the user in the URL")
	f.StringVar(&c.charm, "charm", "", "return results for the charm name")
	f.StringVar(&c.author, "author", "", "return results matching the charm author")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatFindTabular,
	})
}

// Run implements Command.Run.
func (c *findCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	filter := crossmodel.ApplicationOfferFilter{
		ApplicationURL: c.url,
		// TODO(wallyworld): allowed users
		// TODO(wallyworld): charm
		// TODO(wallyworld): user
		// TODO(wallyworld): author
	}
	if c.interfaceName != "" || c.endpoint != "" {
		filter.Endpoints = []crossmodel.EndpointFilterTerm{{
			Interface: c.interfaceName,
			Name:      c.endpoint,
		}}
	}
	found, err := api.FindApplicationOffers(filter)
	if err != nil {
		return err
	}

	output, err := convertFoundOffers(found...)
	if err != nil {
		return err
	}
	if len(output) == 0 {
		return errors.New("no matching application offers found")
	}
	return c.out.Write(ctx, output)
}

// FindAPI defines the API methods that cross model find command uses.
type FindAPI interface {
	Close() error
	FindApplicationOffers(filters ...crossmodel.ApplicationOfferFilter) ([]params.ApplicationOffer, error)
}

// RemoteApplicationResult defines the serialization behaviour of remote application.
// This is used in map-style yaml output where remote application URL is the key.
type RemoteApplicationResult struct {
	// Endpoints is the list of offered application endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

// convertFoundOffers takes any number of api-formatted remote applications and
// creates a collection of ui-formatted applications.
func convertFoundOffers(services ...params.ApplicationOffer) (map[string]RemoteApplicationResult, error) {
	if len(services) == 0 {
		return nil, nil
	}
	output := make(map[string]RemoteApplicationResult, len(services))
	for _, one := range services {
		app := RemoteApplicationResult{Endpoints: convertRemoteEndpoints(one.Endpoints...)}
		output[one.ApplicationURL] = app
	}
	return output, nil
}
