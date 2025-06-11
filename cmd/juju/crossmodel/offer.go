// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

const (
	offerCommandDoc = `
Deployed application endpoints are offered for use by consumers.
By default, the offer is named after the application, unless
an offer name is explicitly specified.
`

	offerCommandExamples = `
    juju offer mysql:db
    juju offer mymodel.mysql:db
    juju offer db2:db hosted-db2
    juju offer db2:db,log hosted-db2
`
)

// NewOfferCommand constructs commands that enables endpoints for export.
func NewOfferCommand() cmd.Command {
	offerCmd := &offerCommand{}
	offerCmd.newAPIFunc = func(ctx context.Context) (OfferAPI, error) {
		return offerCmd.NewApplicationOffersAPI(ctx)
	}
	offerCmd.refreshModels = offerCmd.ControllerCommandBase.RefreshModels
	return modelcmd.WrapController(offerCmd)
}

type offerCommand struct {
	modelcmd.ControllerCommandBase
	newAPIFunc    func(ctx context.Context) (OfferAPI, error)
	refreshModels func(context.Context, jujuclient.ClientStore, string) error
	endpointsSpec string

	// Application stores application name to be offered.
	Application string

	// Endpoints stores a list of endpoints that are being offered.
	Endpoints []string

	// OfferName stores the name of the offer.
	OfferName string

	// QualifiedModelName stores the name of the model hosting the offer.
	QualifiedModelName string
}

// NewApplicationOffersAPI returns an application offers api for the root api endpoint
// that the command returns.
func (c *offerCommand) NewApplicationOffersAPI(ctx context.Context) (*applicationoffers.Client, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

// Info implements Command.Info.
func (c *offerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "offer",
		Purpose:  "Offer application endpoints for use in other models.",
		Args:     "[model-name.]<application-name>:<endpoint-name>[,...] [offer-name]",
		Doc:      offerCommandDoc,
		Examples: offerCommandExamples,
		SeeAlso: []string{
			"consume",
			"integrate",
			"remove-saas",
		},
	})
}

// Init implements Command.Init.
func (c *offerCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("an offer must at least specify application endpoint")
	}
	c.endpointsSpec = args[0]
	argCount := 1
	if len(args) > 1 {
		argCount = 2
		c.OfferName = args[1]
	}
	return cmd.CheckEmpty(args[argCount:])
}

// SetFlags implements Command.SetFlags.
func (c *offerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
}

// Run implements Command.Run.
func (c *offerCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.parseEndpoints(controllerName, c.endpointsSpec); err != nil {
		return err
	}

	if c.QualifiedModelName == "" {
		c.QualifiedModelName, err = c.ClientStore().CurrentModel(controllerName)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				return errors.New("no current model, use juju switch to select a model on which to operate")
			} else {
				return errors.Annotate(err, "cannot load current model")
			}
		}
	}

	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	store := c.ClientStore()
	modelDetails, err := store.ModelByName(controllerName, c.QualifiedModelName)
	if errors.Is(err, errors.NotFound) {
		if err := c.refreshModels(ctx, store, controllerName); err != nil {
			return errors.Annotate(err, "refreshing models cache")
		}
		// Now try again.
		modelDetails, err = store.ModelByName(controllerName, c.QualifiedModelName)
	}
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	if c.OfferName == "" {
		c.OfferName = c.Application
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}
	loggedInUser := accountDetails.User
	// TODO (anastasiamac 2015-11-16) Add a sensible way for user to specify long-ish (at times) description when offering
	results, err := api.Offer(ctx, modelDetails.ModelUUID, c.Application, c.Endpoints, loggedInUser, c.OfferName, "")
	if err != nil {
		return err
	}
	if err := (params.ErrorResults{results}).Combine(); err != nil {
		return err
	}

	unqualifiedModelName, qualifier, err := jujuclient.SplitModelName(c.QualifiedModelName)
	if err != nil {
		return errors.Trace(err)
	}
	url := jujucrossmodel.MakeURL(qualifier, unqualifiedModelName, c.OfferName, "")
	ep := strings.Join(c.Endpoints, ", ")
	ctx.Infof("Application %q endpoints [%s] available at %q", c.Application, ep, url)
	return nil
}

// OfferAPI defines the API methods that the offer command uses.
type OfferAPI interface {
	Close() error
	Offer(ctx context.Context, modelUUID, application string, endpoints []string, owner, offerName, desc string) ([]params.ErrorResult, error)
}

// applicationParse is used to split an application string
// into model, application and endpoint names.
var applicationParse = regexp.MustCompile("/?((?P<model>[^\\.]*)\\.)?(?P<appname>[^:]*)(:(?P<endpoints>.*))?")

func (c *offerCommand) parseEndpoints(controllerName, arg string) error {
	modelNameArg := applicationParse.ReplaceAllString(arg, "$model")
	c.Application = applicationParse.ReplaceAllString(arg, "$appname")
	endpoints := applicationParse.ReplaceAllString(arg, "$endpoints")

	if !strings.Contains(arg, ":") {
		return errors.New(`endpoints must conform to format "<application-name>:<endpoint-name>[,...]" `)
	}
	var (
		modelName string
		err       error
	)
	if modelNameArg != "" && !jujuclient.IsQualifiedModelName(modelNameArg) {
		modelName = modelNameArg
		store := modelcmd.QualifyingClientStore{c.ClientStore()}
		var err error
		c.QualifiedModelName, err = store.QualifiedModelName(controllerName, modelName)
		if err != nil {
			return errors.Trace(err)
		}
	} else if modelNameArg != "" {
		c.QualifiedModelName = modelNameArg
		modelName, _, err = jujuclient.SplitModelName(modelNameArg)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if modelName != "" && !names.IsValidModelName(modelName) {
		return errors.NotValidf(`model name %q`, modelName)
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
