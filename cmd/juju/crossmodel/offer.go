// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"regexp"
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
A vendor offers deployed application endpoints for use by consumers.

Examples:

For local endpoints:
local:/u/<username>/<model-name>/<application-name>

$ juju offer db2:db local:/myapps/db2

For vendor endpoints:
vendor:/u/<username>/<application-name>

$ juju offer db2:db vendor:/u/ibm/hosted-db2
$ juju offer -e prod db2:db,log vendor:/u/ibm/hosted-db2
$ juju offer hosted-db2:db,log vendor:/u/ibm/hosted-db2
`
)

// NewOfferCommand constructs commands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	offerCmd := &offerCommand{}
	offerCmd.newAPIFunc = func() (OfferAPI, error) {
		return offerCmd.NewCrossModelAPI()
	}
	return modelcmd.WrapController(offerCmd)
}

type offerCommand struct {
	CrossModelCommandBase
	newAPIFunc func() (OfferAPI, error)

	// ModelName stores the name of the model containing the application to be offered.
	ModelName string

	// Application stores application name to be offered.
	Application string

	// Endpoints stores a list of endpoints that are being offered.
	Endpoints []string

	// URL stores juju location where these endpoints are offered from.
	URL string
}

// Info implements Command.Info.
func (c *offerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offer",
		Purpose: "Offer application endpoints for use in other models",
		Args:    "<application-name>:<endpoint-name>[,...] <endpoint-url>",
		Doc:     offerCommandDoc,
	}
}

// Init implements Command.Init.
func (c *offerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("an offer must at least specify application endpoint")
	}
	if len(args) < 2 {
		return errors.New("an offer must specify a url")
	}
	if len(args) > 2 {
		return errors.New("an offer can only specify application endpoints and url")
	}

	if err := c.parseEndpoints(args[0]); err != nil {
		return err
	}

	hostedURL := args[1]
	if _, err := crossmodel.ParseApplicationURL(hostedURL); err != nil {
		return errors.Errorf(`hosted url %q is not valid" `, hostedURL)
	}
	c.URL = hostedURL
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *offerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CrossModelCommandBase.SetFlags(f)
}

// Run implements Command.Run.
func (c *offerCommand) Run(_ *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	model, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName)
	if err != nil {
		return err
	}
	// TODO (anastasiamac 2015-11-16) Add a sensible way for user to specify long-ish (at times) description when offering
	results, err := api.Offer(model.ModelUUID, c.Application, c.Endpoints, c.URL, "")
	if err != nil {
		return err
	}
	return params.ErrorResults{results}.Combine()
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(modelUUID, application string, endpoints []string, url string, desc string) ([]params.ErrorResult, error)
}

// applicationParse is used to split an application string
// into model, application and endpoint names.
var applicationParse = regexp.MustCompile("/?((?P<model>[^\\.]*)\\.)?(?P<appname>[^:]*)(:(?P<endpoints>.*))?")

func (c *offerCommand) parseEndpoints(arg string) error {
	c.ModelName = applicationParse.ReplaceAllString(arg, "$model")
	c.Application = applicationParse.ReplaceAllString(arg, "$appname")
	endpoints := applicationParse.ReplaceAllString(arg, "$endpoints")

	if !strings.Contains(arg, ":") {
		return errors.New(`endpoints must conform to format "<application-name>:<endpoint-name>[,...]" `)
	}
	if !names.IsValidApplication(c.Application) {
		return errors.NotValidf(`application name %q`, c.Application)
	}

	c.Endpoints = strings.Split(endpoints, ",")
	if len(endpoints) < 1 || endpoints == "" {
		return errors.Errorf(`specify endpoints for %v" `, c.Application)
	}

	if c.ModelName == "" {
		var err error
		if c.ModelName, err = c.ClientStore().CurrentModel(c.ControllerName()); err != nil {
			return err
		}
	}
	return nil
}
