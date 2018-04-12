// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

var usageConsumeSummary = `
Add a remote offer to the model.`[1:]

var usageConsumeDetails = `
Adds a remote offer to the model. Relations can be created later using "juju relate".

The remote offer is identified by providing a path to the offer:
    [<model owner>/]<model name>.<application name>
        for an application in another model in this controller (if owner isn't specified it's assumed to be the logged-in user)

Examples:
    $ juju consume othermodel.mysql
    $ juju consume owner/othermodel.mysql
    $ juju consume anothercontroller:owner/othermodel.mysql

See also:
    add-relation
    offer`[1:]

// NewConsumeCommand returns a command to add remote offers to
// the model.
func NewConsumeCommand() cmd.Command {
	return modelcmd.Wrap(&consumeCommand{})
}

// consumeCommand adds remote offers to the model without
// relating them to other applications.
type consumeCommand struct {
	modelcmd.ModelCommandBase
	sourceAPI         applicationConsumeDetailsAPI
	targetAPI         applicationConsumeAPI
	remoteApplication string
	applicationAlias  string
}

// Info implements cmd.Command.
func (c *consumeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "consume",
		Args:    "<remote offer path> [<local application name>]",
		Purpose: usageConsumeSummary,
		Doc:     usageConsumeDetails,
	}
}

// Init implements cmd.Command.
func (c *consumeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no remote offer specified")
	}
	c.remoteApplication = args[0]
	if len(args) > 1 {
		if !names.IsValidApplication(args[1]) {
			return errors.Errorf("invalid application name %q", args[1])
		}
		c.applicationAlias = args[1]
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

func (c *consumeCommand) getTargetAPI() (applicationConsumeAPI, error) {
	if c.targetAPI != nil {
		return c.targetAPI, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *consumeCommand) getSourceAPI(url *crossmodel.OfferURL) (applicationConsumeDetailsAPI, error) {
	if c.sourceAPI != nil {
		return c.sourceAPI, nil
	}

	if url.Source == "" {
		var err error
		controllerName, err := c.ControllerName()
		if err != nil {
			return nil, errors.Trace(err)
		}
		url.Source = controllerName
	}
	root, err := c.CommandBase.NewAPIRoot(c.ClientStore(), url.Source, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// Run adds the requested remote offer to the model. Implements
// cmd.Command.
func (c *consumeCommand) Run(ctx *cmd.Context) error {
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}
	url, err := crossmodel.ParseOfferURL(c.remoteApplication)
	if err != nil {
		return errors.Trace(err)
	}
	if url.HasEndpoint() {
		return errors.Errorf("remote offer %q shouldn't include endpoint", c.remoteApplication)
	}
	if url.User == "" {
		url.User = accountDetails.User
		c.remoteApplication = url.Path()
	}
	sourceClient, err := c.getSourceAPI(url)
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceClient.Close()

	consumeDetails, err := sourceClient.GetConsumeDetails(url.AsLocal().String())
	if err != nil {
		return errors.Trace(err)
	}
	// Parse the offer details URL and add the source controller so
	// things like status can show the original source of the offer.
	offerURL, err := crossmodel.ParseOfferURL(consumeDetails.Offer.OfferURL)
	if err != nil {
		return errors.Trace(err)
	}
	offerURL.Source = url.Source
	consumeDetails.Offer.OfferURL = offerURL.String()

	targetClient, err := c.getTargetAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer targetClient.Close()

	arg := crossmodel.ConsumeApplicationArgs{
		Offer:            *consumeDetails.Offer,
		ApplicationAlias: c.applicationAlias,
		Macaroon:         consumeDetails.Macaroon,
	}
	if consumeDetails.ControllerInfo != nil {
		controllerTag, err := names.ParseControllerTag(consumeDetails.ControllerInfo.ControllerTag)
		if err != nil {
			return errors.Trace(err)
		}
		arg.ControllerInfo = &crossmodel.ControllerInfo{
			ControllerTag: controllerTag,
			Alias:         consumeDetails.ControllerInfo.Alias,
			Addrs:         consumeDetails.ControllerInfo.Addrs,
			CACert:        consumeDetails.ControllerInfo.CACert,
		}
	}
	localName, err := targetClient.Consume(arg)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("Added %s as %s", c.remoteApplication, localName)
	return nil
}

type applicationConsumeAPI interface {
	Close() error
	Consume(crossmodel.ConsumeApplicationArgs) (string, error)
}

type applicationConsumeDetailsAPI interface {
	Close() error
	GetConsumeDetails(string) (params.ConsumeOfferDetails, error)
}
