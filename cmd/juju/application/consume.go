// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/rpc/params"
)

var usageConsumeSummary = `
Add a remote offer to the model.`[1:]

var usageConsumeDetails = `
Adds a remote offer to the model. Relations can be created later using "juju relate".

The path to the remote offer is formatted as follows:

    [<controller name>:][<model owner>/]<model name>.<application name>
        
If the controller name is omitted, Juju will use the currently active
controller. Similarly, if the model owner is omitted, Juju will use the user
that is currently logged in to the controller providing the offer.
`[1:]

const usageConsumeExamples = `
    juju consume othermodel.mysql
    juju consume owner/othermodel.mysql
    juju consume anothercontroller:owner/othermodel.mysql
`

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
	return jujucmd.Info(&cmd.Info{
		Name:     "consume",
		Args:     "<remote offer path> [<local application name>]",
		Purpose:  usageConsumeSummary,
		Doc:      usageConsumeDetails,
		Examples: usageConsumeExamples,
		SeeAlso: []string{
			"integrate",
			"offer",
			"remove-saas",
		},
	})
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

func (c *consumeCommand) getTargetAPI(ctx context.Context) (applicationConsumeAPI, error) {
	if c.targetAPI != nil {
		return c.targetAPI, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *consumeCommand) getSourceAPI(ctx context.Context, url *crossmodel.OfferURL) (applicationConsumeDetailsAPI, error) {
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
	root, err := c.CommandBase.NewAPIRoot(ctx, c.ClientStore(), url.Source, "")
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
		return errors.Errorf("saas offer %q shouldn't include endpoint", c.remoteApplication)
	}
	if url.ModelNamespace == "" {
		url.ModelNamespace = accountDetails.User
		c.remoteApplication = url.Path()
	}
	sourceClient, err := c.getSourceAPI(ctx, url)
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceClient.Close()

	consumeDetails, err := sourceClient.GetConsumeDetails(ctx, url.AsLocal().String())
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

	targetClient, err := c.getTargetAPI(ctx)
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
			ControllerUUID: controllerTag.Id(),
			Alias:          consumeDetails.ControllerInfo.Alias,
			Addrs:          consumeDetails.ControllerInfo.Addrs,
			CACert:         consumeDetails.ControllerInfo.CACert,
		}
	}
	localName, err := targetClient.Consume(ctx, arg)
	if err != nil {
		return block.ProcessBlockedError(errors.Annotatef(err, "could not consume %v", url.AsLocal().String()), block.BlockChange)
	}
	ctx.Infof("Added %s as %s", c.remoteApplication, localName)
	return nil
}

type applicationConsumeAPI interface {
	Close() error
	Consume(context.Context, crossmodel.ConsumeApplicationArgs) (string, error)
}

type applicationConsumeDetailsAPI interface {
	Close() error
	GetConsumeDetails(context.Context, string) (params.ConsumeOfferDetails, error)
}
