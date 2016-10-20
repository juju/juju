// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

const (
	offerCommandDoc = `
A vendor offers deployed application endpoints for use by consumers in their own models.

Examples:
$ juju offer db2:db 
$ juju offer db2:db local:db2
$ juju offer -e prod db2:db,log vendor:/u/ibm/hosted-db2
$ juju offer hosted-db2:db,log vendor:/u/ibm/hosted-db2 --to public
`
	offerCommandAgs = `
<application-name>:<endpoint-name>[,...] [<endpoint-url>] [--to <user-ident>,...]
where 

endpoint-url    For local endpoints:
                local:/u/<username>/<model-name>/<application-name>

                    $ juju offer db2:db 
                    
                endpoint “db” available at local:/u/user-name/model-name/hosted-db2
                    
                For vendor endpoints:
                vendor:/u/<username>/<application-name>
                    
                    $ juju offer db2:db vendor:/u/ibm/hosted-db2

                endpoint “db” available at vendor:/u/ibm/hosted-db2     
`
)

// NewOfferCommand constructs commands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	offerCmd := &offerCommand{}
	offerCmd.newAPIFunc = func() (OfferAPI, error) {
		return offerCmd.NewCrossModelAPI()
	}
	return modelcmd.Wrap(offerCmd)
}

type offerCommand struct {
	CrossModelCommandBase
	newAPIFunc func() (OfferAPI, error)

	// Application stores application name to be offered.
	Application string

	// Endpoints stores a list of endpoints that are being offered.
	Endpoints []string

	// URL stores juju location where these endpoints are offered from.
	URL string

	// Users stores a list of users that these endpoints are offered to.
	Users []string
}

// Info implements Command.Info.
func (c *offerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offer",
		Purpose: "Offer application endpoints for use in other models",
		Args:    offerCommandAgs,
		Doc:     offerCommandDoc,
	}
}

// Init implements Command.Init.
func (c *offerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("an offer must at least specify application endpoint")
	}
	if len(args) > 2 {
		return errors.New("an offer can only specify application endpoints and url")
	}

	if err := c.parseEndpoints(args[0]); err != nil {
		return err
	}

	if len(args) == 2 {
		hostedURL := args[1]
		if _, err := crossmodel.ParseApplicationURL(hostedURL); err != nil {
			return errors.Errorf(`hosted url %q is not valid" `, hostedURL)
		}
		c.URL = hostedURL
	}

	return nil
}

// SetFlags implements Command.SetFlags.
func (c *offerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
	f.Var(cmd.NewStringsValue(nil, &c.Users), "to", "users that these endpoints are offered to")
}

// Run implements Command.Run.
func (c *offerCommand) Run(_ *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	userTags := make([]string, len(c.Users))
	for i, user := range c.Users {
		if !names.IsValidUser(user) {
			return errors.NotValidf(`user name %q`, user)
		}
		userTags[i] = names.NewUserTag(user).String()
	}

	// TODO (anastasiamac 2015-11-16) Add a sensible way for user to specify long-ish (at times) description when offering
	results, err := api.Offer(c.Application, c.Endpoints, c.URL, userTags, "")
	if err != nil {
		return err
	}
	return params.ErrorResults{results}.Combine()
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(application string, endpoints []string, url string, users []string, desc string) ([]params.ErrorResult, error)
}

func (c *offerCommand) parseEndpoints(arg string) error {
	parts := strings.SplitN(arg, ":", -1)

	if len(parts) != 2 {
		return errors.New(`endpoints must conform to format "<application-name>:<endpoint-name>[,...]" `)
	}

	ApplicationName := parts[0]
	if !names.IsValidApplication(ApplicationName) {
		return errors.NotValidf(`application name %q`, ApplicationName)
	}
	c.Application = ApplicationName

	endpoints := strings.SplitN(parts[1], ",", -1)
	if len(endpoints) < 1 || endpoints[0] == "" {
		return errors.Errorf(`specify endpoints for %v" `, ApplicationName)
	}

	c.Endpoints = endpoints
	if c.URL == "" {
		// TODO (wallyworld) - do this serverside after results struct is changed
		account, err := c.ClientStore().AccountDetails(c.ControllerName())
		if err != nil {
			return err
		}
		c.URL = fmt.Sprintf("local:/u/%s/%s/%s", account.User, c.ModelName(), ApplicationName)
	}
	return nil
}
