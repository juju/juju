// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
)

// NewRemoveOfferCommand returns a command used to remove a specified offer.
func NewRemoveOfferCommand() cmd.Command {
	removeCmd := &removeCommand{}
	removeCmd.newAPIFunc = func(ctx context.Context, controllerName string) (RemoveAPI, error) {
		return removeCmd.NewApplicationOffersAPI(ctx, controllerName)
	}
	return modelcmd.WrapController(removeCmd)
}

type removeCommand struct {
	modelcmd.ControllerCommandBase
	newAPIFunc  func(context.Context, string) (RemoveAPI, error)
	offers      []string
	offerSource string

	assumeYes bool
	force     bool
}

const destroyOfferDoc = `
Remove one or more application offers.

If the --force option is specified, any existing relations to the
offer will also be removed.

Offers to remove are normally specified by their URL.
It's also possible to specify just the offer name, in which case
the offer is considered to reside in the current model.
`

const destroyOfferExamples = `
    juju remove-offer staging/mymodel.hosted-mysql
    juju remove-offer staging/mymodel.hosted-mysql --force
    juju remove-offer hosted-mysql
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-offer",
		Args:     "<offer-url> ...",
		Purpose:  "Removes one or more offers specified by their URL.",
		Doc:      destroyOfferDoc,
		Examples: destroyOfferExamples,
		SeeAlso: []string{
			"find-offers",
			"offer",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "remove the offer as well as any relations to the offer")
	f.BoolVar(&c.assumeYes, "y", false, "Do not prompt for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no offers specified")
	}
	c.offers = args
	return nil
}

// RemoveAPI defines the API methods that the remove offer command uses.
type RemoveAPI interface {
	Close() error
	DestroyOffers(ctx context.Context, force bool, offerURLs ...string) error
}

// NewApplicationOffersAPI returns an application offers api.
func (c *removeCommand) NewApplicationOffersAPI(ctx context.Context, controllerName string) (*applicationoffers.Client, error) {
	root, err := c.CommandBase.NewAPIRoot(ctx, c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

var removeOfferMsg = `
WARNING! This command will remove offers: %v
This includes all relations to those offers.
`[1:]

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	// Allow for the offers to be specified by name rather than a full URL.
	// In that case, we need to assume the offer resides in the current model.
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	currentModel, err := c.ClientStore().CurrentModel(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	var invalidOffers []string
	for i, urlStr := range c.offers {
		url, err := c.parseOfferURL(controllerName, currentModel, urlStr)
		if err != nil {
			return errors.Trace(err)
		}
		c.offers[i] = url.String()
		if c.offerSource == "" {
			c.offerSource = url.Source
		}
		if c.offerSource != url.Source {
			return errors.New("all offer URLs must use the same controller")
		}
		if strings.Contains(url.ApplicationName, ":") {
			invalidOffers = append(invalidOffers, " -"+c.offers[i])
		}
	}

	if len(invalidOffers) > 0 {
		return errors.Errorf("These offers contain endpoints. Only specify the offer name itself.\n%v", strings.Join(invalidOffers, "\n"))
	}

	if c.offerSource == "" {
		c.offerSource = controllerName
	}

	if !c.assumeYes && c.force {
		fmt.Fprintf(ctx.Stderr, removeOfferMsg, strings.Join(c.offers, ", "))

		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "offer removal")
		}
	}

	api, err := c.newAPIFunc(ctx, c.offerSource)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = api.DestroyOffers(ctx, c.force, c.offers...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}

func (c *removeCommand) parseOfferURL(controllerName, currentModel, urlStr string) (*crossmodel.OfferURL, error) {
	url, err := crossmodel.ParseOfferURL(urlStr)
	if err == nil {
		return url, nil
	}
	if !names.IsValidApplication(urlStr) {
		return nil, errors.Trace(err)
	}
	store := c.ClientStore()
	return makeURLFromCurrentModel(store, controllerName, c.offerSource, currentModel, urlStr)
}

func makeURLFromCurrentModel(
	store jujuclient.ClientStore, controllerName, offerSource, modelName, offerName string,
) (*crossmodel.OfferURL, error) {
	// We may have just been given an offer name.
	// Try again with the current model as the host model.
	url := &crossmodel.OfferURL{
		Source:          offerSource,
		ApplicationName: offerName,
	}
	if url.ModelName == "" {
		if jujuclient.IsQualifiedModelName(modelName) {
			modelName, qualifier, err := jujuclient.SplitFullyQualifiedModelName(modelName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			url.ModelQualifier = qualifier
			url.ModelName = modelName
		} else {
			url.ModelName = modelName
		}
	}

	if url.ModelQualifier == "" {
		accountDetails, err := store.AccountDetails(controllerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		qualifier := model.QualifierFromUserTag(names.NewUserTag(accountDetails.User))
		url.ModelQualifier = qualifier.String()
	}
	return url, nil
}
