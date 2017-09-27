// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
)

// NewRemoveOfferCommand returns a command used to remove a specified offer.
func NewRemoveOfferCommand() cmd.Command {
	removeCmd := &removeCommand{}
	removeCmd.newAPIFunc = func(controllerName string) (RemoveAPI, error) {
		return removeCmd.NewApplicationOffersAPI(controllerName)
	}
	return modelcmd.WrapController(removeCmd)
}

type removeCommand struct {
	modelcmd.ControllerCommandBase
	newAPIFunc  func(string) (RemoveAPI, error)
	offers      []string
	offerSource string
}

const destroyOfferDoc = `
Remove one or more application offers.

Examples:

    juju remove-offer prod.model/hosted-mysql

See also:
    find-endpoints
    offer
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-offer",
		Args:    "<offer-url> ...",
		Purpose: "Removes one or more offers specified by their URL.",
		Doc:     destroyOfferDoc,
	}
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no offers specified")
	}
	for _, urlStr := range args {
		url, err := crossmodel.ParseOfferURL(urlStr)
		if err != nil {
			return errors.Trace(err)
		}
		if c.offerSource == "" {
			c.offerSource = url.Source
		}
		if c.offerSource != url.Source {
			return errors.New("all offer URLs must use the same controller")
		}
	}
	c.offers = args
	return nil
}

// RemoveAPI defines the API methods that the remove offer command uses.
type RemoveAPI interface {
	Close() error
	DestroyOffers(offerURLs ...string) error
}

// NewApplicationOffersAPI returns an application offers api.
func (c *removeCommand) NewApplicationOffersAPI(controllerName string) (*applicationoffers.Client, error) {
	root, err := c.CommandBase.NewAPIRoot(c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	if c.offerSource == "" {
		controllerName, err := c.ControllerName()
		if err != nil {
			return errors.Trace(err)
		}
		c.offerSource = controllerName
	}
	api, err := c.newAPIFunc(c.offerSource)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.DestroyOffers(c.offers...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}
