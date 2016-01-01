// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/gnuflag"

	"fmt"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/model/crossmodel"
)

const findCommandDoc = `
Find which offered service endpoints are available to the current user.

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
// allows to find offered service endpoints.
func NewFindEndpointsCommand() cmd.Command {
	findCmd := &findCommand{}
	findCmd.newAPIFunc = func() (FindAPI, error) {
		return findCmd.NewCrossModelAPI()
	}
	return envcmd.Wrap(findCmd)
}

// Init implements Command.Init.
func (c *findCommand) Init(args []string) (err error) {
	if len(args) > 1 {
		return errors.Errorf("unrecognized args: %q", args[1:])
	}

	if len(args) == 1 {
		if c.url != "" {
			return errors.New("URL term cannot be specified twice")
		}
		c.url = args[0]
	}
	if c.url != "" {
		if _, err := crossmodel.ParseServiceURLParts(c.url); err != nil {
			return err
		}
	}
	return nil
}

// Info implements Command.Info.
func (c *findCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "find-endpoints",
		Purpose: "find offered service endpoints",
		Doc:     findCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	f.StringVar(&c.url, "url", "", "service URL")
	f.StringVar(&c.interfaceName, "interface", "", "service URL")
	f.StringVar(&c.endpoint, "endpoint", "", "service URL")
	f.StringVar(&c.user, "user", "", "service URL")
	f.StringVar(&c.charm, "charm", "", "service URL")
	f.StringVar(&c.author, "author", "", "service URL")
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

	filter := crossmodel.ServiceOfferFilter{
		ServiceOffer: crossmodel.ServiceOffer{
			ServiceURL: c.url,
		},
		// TODO(wallyworld): allowed users
		// TODO(wallyworld): charm
		// TODO(wallyworld): user
		// TODO(wallyworld): author
	}
	if c.interfaceName != "" || c.endpoint != "" {
		filter.Endpoints = []charm.Relation{{
			Interface: c.interfaceName,
			Name:      c.endpoint,
		}}
	}
	found, err := api.FindServiceOffers(filter)
	if err != nil {
		return err
	}

	output, err := convertFoundServices(found...)
	if err != nil {
		return err
	}
	if len(output) == 0 {
		fmt.Fprintln(ctx.Stdout, "no matching service offers found")
		return nil
	}
	return c.out.Write(ctx, output)
}

// FindAPI defines the API methods that cross model find command uses.
type FindAPI interface {
	Close() error
	FindServiceOffers(filters ...crossmodel.ServiceOfferFilter) ([]params.ServiceOffer, error)
}

// RemoteServiceResult defines the serialization behaviour of remote service.
// This is used in map-style yaml output where remote service URL is the key.
type RemoteServiceResult struct {
	// Endpoints is the list of offered service endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`
}

// convertFoundServices takes any number of api-formatted remote services and
// creates a collection of ui-formatted services.
func convertFoundServices(services ...params.ServiceOffer) (map[string]RemoteServiceResult, error) {
	if len(services) == 0 {
		return nil, nil
	}
	output := make(map[string]RemoteServiceResult, len(services))
	for _, one := range services {
		service := RemoteServiceResult{Endpoints: convertRemoteEndpoints(one.Endpoints...)}
		output[one.ServiceURL] = service
	}
	return output, nil
}
