// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/api/jujuclient"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	coremodel "github.com/juju/juju/core/model"
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

	force    bool
	noPrompt bool
}

const destroyOfferDoc = `
Remove one or more application offers.

If an offer has active connections, Juju will ask for confirmation before
removing the offer and the relations to it unless --no-prompt is used.

Use --force to request forced offer removal from the controller.

Offers to remove are normally specified by their URL.
It's also possible to specify just the offer name, in which case
the offer is considered to reside in the current model.
`

const destroyOfferExamples = `
    juju remove-offer staging/mymodel.hosted-mysql
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
	f.BoolVar(&c.force, "force", false, "Force remove the offer")
	f.BoolVar(&c.noPrompt, "no-prompt", false, "Do not prompt for confirmation")
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
	ListOffers(ctx context.Context, filters ...crossmodel.ApplicationOfferFilter) ([]*crossmodel.ApplicationOfferDetails, error)
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
Any existing relations to those offers will also be removed.
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
	parsedOffers := make([]crossmodel.OfferURL, len(c.offers))
	for i, urlStr := range c.offers {
		url, err := c.parseOfferURL(controllerName, currentModel, urlStr)
		if err != nil {
			return errors.Trace(err)
		}
		parsedOffers[i] = url
		c.offers[i] = url.String()
		if c.offerSource == "" {
			c.offerSource = url.Source
		}
		if c.offerSource != url.Source {
			return errors.New("all offer URLs must use the same controller")
		}
		if strings.Contains(url.Name, ":") {
			invalidOffers = append(invalidOffers, " -"+c.offers[i])
		}
	}

	if len(invalidOffers) > 0 {
		return errors.Errorf("These offers contain endpoints. Only specify the offer name itself.\n%v", strings.Join(invalidOffers, "\n"))
	}

	if c.offerSource == "" {
		c.offerSource = controllerName
	}

	api, err := c.newAPIFunc(ctx, c.offerSource)
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	err = c.resolveOfferRemoval(ctx, api, parsedOffers)
	if err != nil {
		return errors.Trace(err)
	}

	err = api.DestroyOffers(ctx, c.force, c.offers...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}

func (c *removeCommand) resolveOfferRemoval(
	ctx *cmd.Context,
	api RemoveAPI,
	offers []crossmodel.OfferURL,
) error {
	connectedOffers, err := c.connectedOffers(ctx, api, offers)
	if err != nil {
		return errors.Trace(err)
	}
	if len(connectedOffers) == 0 {
		return nil
	}

	if err := c.confirmOfferRemoval(ctx, connectedOffers); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *removeCommand) confirmOfferRemoval(ctx *cmd.Context, offers []string) error {
	if c.noPrompt {
		return nil
	}

	fmt.Fprintf(ctx.Stderr, removeOfferMsg, strings.Join(offers, ", "))
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "offer removal")
	}
	return nil
}

func (c *removeCommand) connectedOffers(
	ctx context.Context,
	api RemoveAPI,
	offers []crossmodel.OfferURL,
) ([]string, error) {
	filters := make([]crossmodel.ApplicationOfferFilter, len(offers))
	userOffers := make(map[string]string, len(offers))
	for i, offer := range offers {
		localURL := offer.AsLocal().String()
		filters[i] = crossmodel.ApplicationOfferFilter{
			ModelQualifier: coremodel.Qualifier(offer.ModelQualifier),
			ModelName:      offer.ModelName,
			OfferName:      fmt.Sprintf("^%v$", regexp.QuoteMeta(offer.Name)),
		}
		userOffers[localURL] = c.offers[i]
	}

	listedOffers, err := api.ListOffers(ctx, filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	connected := make([]string, 0)
	for _, offer := range listedOffers {
		if len(offer.Connections) == 0 {
			continue
		}
		userOffer, ok := userOffers[offer.OfferURL]
		if !ok {
			continue
		}
		connected = append(connected, userOffer)
	}

	return connected, nil
}

func (c *removeCommand) parseOfferURL(controllerName, currentModel, urlStr string) (crossmodel.OfferURL, error) {
	url, err := crossmodel.ParseOfferURL(urlStr)
	if err == nil {
		return url, nil
	}
	if !names.IsValidApplication(urlStr) {
		return crossmodel.OfferURL{}, errors.Trace(err)
	}
	store := c.ClientStore()
	return makeURLFromCurrentModel(store, controllerName, c.offerSource, currentModel, urlStr)
}

func makeURLFromCurrentModel(
	store jujuclient.ClientStore, controllerName, offerSource, modelName, offerName string,
) (crossmodel.OfferURL, error) {
	// We may have just been given an offer name.
	// Try again with the current model as the host model.
	url := crossmodel.OfferURL{
		Source: offerSource,
		Name:   offerName,
	}
	if url.ModelName == "" {
		if jujuclient.IsQualifiedModelName(modelName) {
			unqualifiedName, owner, err := jujuclient.SplitFullyQualifiedModelName(modelName)
			if err != nil {
				return crossmodel.OfferURL{}, errors.Trace(err)
			}
			url.ModelName = unqualifiedName
			url.ModelQualifier = owner
		} else {
			url.ModelName = modelName
		}
	}

	if url.ModelQualifier == "" {
		accountDetails, err := store.AccountDetails(controllerName)
		if err != nil {
			return crossmodel.OfferURL{}, errors.Trace(err)
		}
		url.ModelQualifier = accountDetails.User
	}
	return url, nil
}
