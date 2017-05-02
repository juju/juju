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
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/jujuclient"
)

const (
	offerCommandDoc = `
Deployed application endpoints are offered for use by consumers.
By default, the offer is named after the application, unless
an offer name is explicitly specified.

Examples:

$ juju offer mysql:db
$ juju offer db2:db hosted-db2
$ juju offer db2:db,log hosted-db2

See also:
    consume
    relate
`
)

// NewOfferCommand constructs commands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	offerCmd := &offerCommand{}
	offerCmd.newAPIFunc = func() (OfferAPI, error) {
		return offerCmd.NewApplicationOffersAPI()
	}
	return modelcmd.Wrap(offerCmd)
}

type offerCommand struct {
	ApplicationOffersCommandBase
	newAPIFunc func() (OfferAPI, error)

	// Application stores application name to be offered.
	Application string

	// Endpoints stores a list of endpoints that are being offered.
	Endpoints []string

	// OfferName stores the name of the offer
	OfferName string
}

// Info implements Command.Info.
func (c *offerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "offer",
		Purpose: "Offer application endpoints for use in other models",
		Args:    "<application-name>:<endpoint-name>[,...] [offer-name]",
		Doc:     offerCommandDoc,
	}
}

// Init implements Command.Init.
func (c *offerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("an offer must at least specify application endpoint")
	}
	if err := c.parseEndpoints(args[0]); err != nil {
		return err
	}
	argCount := 1
	if len(args) > 1 {
		argCount = 2
		c.OfferName = args[1]
	}
	if c.OfferName == "" {
		c.OfferName = c.Application
	}
	return cmd.CheckEmpty(args[argCount:])
}

// SetFlags implements Command.SetFlags.
func (c *offerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ApplicationOffersCommandBase.SetFlags(f)
}

// Run implements Command.Run.
func (c *offerCommand) Run(ctx *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	// TODO (anastasiamac 2015-11-16) Add a sensible way for user to specify long-ish (at times) description when offering
	results, err := api.Offer(c.Application, c.Endpoints, c.OfferName, "")
	if err != nil {
		return errors.Trace(err)
	}
	if err := (params.ErrorResults{results}).Combine(); err != nil {
		return errors.Trace(err)
	}
	modelName, err := c.ModelName()
	if err != nil {
		return errors.Trace(err)
	}
	var unqualifiedModelName, owner string
	if jujuclient.IsQualifiedModelName(modelName) {
		var ownerTag names.UserTag
		unqualifiedModelName, ownerTag, err = jujuclient.SplitModelName(modelName)
		if err != nil {
			return errors.Trace(err)
		}
		owner = ownerTag.Name()
	} else {
		unqualifiedModelName = modelName
		account, err := c.CurrentAccountDetails()
		if err != nil {
			return errors.Trace(err)
		}
		owner = account.User
	}
	url := jujucrossmodel.MakeURL(owner, unqualifiedModelName, c.OfferName, "")
	ep := strings.Join(c.Endpoints, ", ")
	ctx.Infof("Application %q endpoints [%s] available at %q", c.Application, ep, url)
	return nil
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(application string, endpoints []string, offerName string, desc string) ([]params.ErrorResult, error)
}

// applicationParse is used to split an application string
// into model, application and endpoint names.
var applicationParse = regexp.MustCompile("/?((?P<model>[^\\.]*)\\.)?(?P<appname>[^:]*)(:(?P<endpoints>.*))?")

func (c *offerCommand) parseEndpoints(arg string) error {
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

	return nil
}
