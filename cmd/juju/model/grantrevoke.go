// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/client/applicationoffers"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
)

var usageGrantSummary = `
Grants access level to a Juju user for a model, controller, or application offer.`[1:]

func filterAccessLevels(accessLevels []permission.Access, filter func(permission.Access) error) []string {
	ret := []string{}
	for _, accessLevel := range accessLevels {
		if filter(accessLevel) == nil {
			ret = append(ret, string(accessLevel))
		}
	}
	return ret
}

var validAccessLevels = `
Valid access levels for models are:
    `[1:] + strings.Join(filterAccessLevels(permission.AllAccessLevels, permission.ValidateModelAccess), "\n    ") + `

Valid access levels for controllers are:
    ` + strings.Join(filterAccessLevels(permission.AllAccessLevels, permission.ValidateControllerAccess), "\n    ") + `

Valid access levels for application offers are:
    ` + strings.Join(filterAccessLevels(permission.AllAccessLevels, permission.ValidateOfferAccess), "\n    ")

var usageGrantDetails = `
By default, the controller is the current controller.

Users with read access are limited in what they can do with models:
` + "`juju models`, `juju machines`, and `juju status`" + `.

`[1:] + validAccessLevels

const usageGrantExamples = `
Grant user 'joe' 'read' access to model 'mymodel':

    juju grant joe read mymodel

Grant user 'jim' 'write' access to model 'mymodel':

    juju grant jim write mymodel

Grant user 'sam' 'read' access to models 'model1' and 'model2':

    juju grant sam read model1 model2

Grant user 'joe' 'read' access to application offer 'fred/prod.hosted-mysql':

    juju grant joe read fred/prod.hosted-mysql

Grant user 'jim' 'consume' access to application offer 'fred/prod.hosted-mysql':

    juju grant jim consume fred/prod.hosted-mysql

Grant user 'sam' 'read' access to application offers 'fred/prod.hosted-mysql' and 'mary/test.hosted-mysql':

    juju grant sam read fred/prod.hosted-mysql mary/test.hosted-mysql

`

var usageRevokeSummary = `
Revokes access from a Juju user for a model, controller, or application offer.`[1:]

var usageRevokeDetails = `
By default, the controller is the current controller.

Revoking write access, from a user who has that permission, will leave
that user with read access. Revoking read access, however, also revokes
write access.

`[1:] + validAccessLevels

const usageRevokeExamples = `
Revoke 'read' (and 'write') access from user 'joe' for model 'mymodel':

    juju revoke joe read mymodel

Revoke 'write' access from user 'sam' for models 'model1' and 'model2':

    juju revoke sam write model1 model2

Revoke 'read' (and 'write') access from user 'joe' for application offer 'fred/prod.hosted-mysql':

    juju revoke joe read fred/prod.hosted-mysql

Revoke 'consume' access from user 'sam' for models 'fred/prod.hosted-mysql' and 'mary/test.hosted-mysql':

    juju revoke sam consume fred/prod.hosted-mysql mary/test.hosted-mysql
`

type accessCommand struct {
	modelcmd.ControllerCommandBase

	User       string
	ModelNames []string
	OfferURLs  []*crossmodel.OfferURL
	Access     string
}

// Init implements cmd.Command.
func (c *accessCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no user specified")
	}

	if len(args) < 2 {
		return errors.New("no permission level specified")
	}

	c.User = args[0]
	c.Access = args[1]
	// The remaining args are either model names or offer names.
	for _, arg := range args[2:] {
		url, err := crossmodel.ParseOfferURL(arg)
		if err == nil {
			c.OfferURLs = append(c.OfferURLs, url)
			continue
		}
		maybeModelName := arg
		if jujuclient.IsQualifiedModelName(maybeModelName) {
			var err error
			maybeModelName, _, err = jujuclient.SplitModelName(maybeModelName)
			if err != nil {
				return errors.Annotatef(err, "validating model name %q", maybeModelName)
			}
		}
		if !names.IsValidModelName(maybeModelName) {
			return errors.NotValidf("model name %q", maybeModelName)
		}
		c.ModelNames = append(c.ModelNames, arg)
	}
	if len(c.ModelNames) > 0 && len(c.OfferURLs) > 0 {
		return errors.New("either specify model names or offer URLs but not both")
	}

	if len(c.ModelNames) > 0 || len(c.OfferURLs) > 0 {
		if err := permission.ValidateControllerAccess(permission.Access(c.Access)); err == nil {
			return errors.Errorf("You have specified a controller access permission %q.\n"+
				"If you intended to change controller access, do not specify any model names or offer URLs.\n"+
				"See 'juju help grant'.", c.Access)
		}
	}
	if len(c.ModelNames) > 0 {
		return permission.ValidateModelAccess(permission.Access(c.Access))
	}
	if len(c.OfferURLs) > 0 {
		return permission.ValidateOfferAccess(permission.Access(c.Access))
	}
	if err := permission.ValidateModelAccess(permission.Access(c.Access)); err == nil {
		return errors.Errorf("You have specified a model access permission %q.\n"+
			"If you intended to change model access, you need to specify one or more model names.\n"+
			"See 'juju help grant'.", c.Access)
	}
	return nil
}

// NewGrantCommand returns a new grant command.
func NewGrantCommand() cmd.Command {
	return modelcmd.WrapController(&grantCommand{})
}

// grantCommand represents the command to grant a user access to one or more models.
type grantCommand struct {
	accessCommand
	modelsApi GrantModelAPI
	offersApi GrantOfferAPI
}

// Info implements Command.Info.
func (c *grantCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "grant",
		Args:     "<user name> <permission> [<model name> ... | <offer url> ...]",
		Purpose:  usageGrantSummary,
		Doc:      usageGrantDetails,
		Examples: usageGrantExamples,
		SeeAlso: []string{
			"revoke",
			"add-user",
			"grant-cloud",
		},
	})
}

func (c *grantCommand) getModelAPI(ctx context.Context) (GrantModelAPI, error) {
	if c.modelsApi != nil {
		return c.modelsApi, nil
	}
	return c.NewModelManagerAPIClient(ctx)
}

func (c *grantCommand) getControllerAPI(ctx context.Context) (GrantControllerAPI, error) {
	return c.NewControllerAPIClient(ctx)
}

func (c *grantCommand) getOfferAPI(ctx context.Context) (GrantOfferAPI, error) {
	if c.offersApi != nil {
		return c.offersApi, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// GrantModelAPI defines the API functions used by the grant command.
type GrantModelAPI interface {
	Close() error
	GrantModel(ctx context.Context, user, access string, modelUUIDs ...string) error
}

// GrantControllerAPI defines the API functions used by the grant command.
type GrantControllerAPI interface {
	Close() error
	GrantController(ctx context.Context, user, access string) error
}

// GrantOfferAPI defines the API functions used by the grant command.
type GrantOfferAPI interface {
	Close() error
	GrantOffer(ctx context.Context, user, access string, offerURLs ...string) error
}

// Run implements cmd.Command.
func (c *grantCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel(ctx)
	}
	if len(c.OfferURLs) > 0 {
		if err := setUnsetUsers(c, c.OfferURLs); err != nil {
			return errors.Trace(err)
		}
		return c.runForOffers(ctx)
	}
	return c.runForController(ctx)
}

func (c *grantCommand) runForController(ctx context.Context) error {
	client, err := c.getControllerAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.GrantController(ctx, c.User, c.Access), block.BlockChange)
}

func (c *grantCommand) runForModel(ctx context.Context) error {
	client, err := c.getModelAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(ctx, c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.GrantModel(ctx, c.User, c.Access, models...), block.BlockChange)
}

func (c *grantCommand) runForOffers(ctx context.Context) error {
	client, err := c.getOfferAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	urls := make([]string, len(c.OfferURLs))
	for i, url := range c.OfferURLs {
		urls[i] = url.String()
	}
	err = client.GrantOffer(ctx, c.User, c.Access, urls...)
	return block.ProcessBlockedError(err, block.BlockChange)
}

// NewRevokeCommand returns a new revoke command.
func NewRevokeCommand() cmd.Command {
	return modelcmd.WrapController(&revokeCommand{})
}

// revokeCommand revokes a user's access to models.
type revokeCommand struct {
	accessCommand
	modelsApi RevokeModelAPI
	offersApi RevokeOfferAPI
}

// Info implements cmd.Command.
func (c *revokeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "revoke",
		Args:     "<user name> <permission> [<model name> ... | <offer url> ...]",
		Purpose:  usageRevokeSummary,
		Doc:      usageRevokeDetails,
		Examples: usageRevokeExamples,
		SeeAlso: []string{
			"grant",
		},
	})
}

func (c *revokeCommand) getModelAPI(ctx context.Context) (RevokeModelAPI, error) {
	if c.modelsApi != nil {
		return c.modelsApi, nil
	}
	return c.NewModelManagerAPIClient(ctx)
}

func (c *revokeCommand) getControllerAPI(ctx context.Context) (RevokeControllerAPI, error) {
	return c.NewControllerAPIClient(ctx)
}

func (c *revokeCommand) getOfferAPI(ctx context.Context) (RevokeOfferAPI, error) {
	if c.offersApi != nil {
		return c.offersApi, nil
	}
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// RevokeModelAPI defines the API functions used by the revoke command.
type RevokeModelAPI interface {
	Close() error
	RevokeModel(ctx context.Context, user, access string, modelUUIDs ...string) error
}

// RevokeControllerAPI defines the API functions used by the revoke command.
type RevokeControllerAPI interface {
	Close() error
	RevokeController(ctx context.Context, user, access string) error
}

// RevokeOfferAPI defines the API functions used by the revoke command.
type RevokeOfferAPI interface {
	Close() error
	RevokeOffer(ctx context.Context, user, access string, offerURLs ...string) error
}

// Run implements cmd.Command.
func (c *revokeCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel(ctx)
	}
	if len(c.OfferURLs) > 0 {
		if err := setUnsetUsers(c, c.OfferURLs); err != nil {
			return errors.Trace(err)
		}
		return c.runForOffers(ctx)
	}
	return c.runForController(ctx)
}

func (c *revokeCommand) runForController(ctx context.Context) error {
	client, err := c.getControllerAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.RevokeController(ctx, c.User, c.Access), block.BlockChange)
}

func (c *revokeCommand) runForModel(ctx context.Context) error {
	client, err := c.getModelAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(ctx, c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.RevokeModel(ctx, c.User, c.Access, models...), block.BlockChange)
}

type accountDetailsGetter interface {
	CurrentAccountDetails() (*jujuclient.AccountDetails, error)
}

// setUnsetUsers sets any empty user entries in the given offer URLs
// to the currently logged in user.
func setUnsetUsers(c accountDetailsGetter, offerURLs []*crossmodel.OfferURL) error {
	var currentAccountDetails *jujuclient.AccountDetails
	for _, url := range offerURLs {
		if url.ModelQualifier != "" {
			continue
		}
		if currentAccountDetails == nil {
			var err error
			currentAccountDetails, err = c.CurrentAccountDetails()
			if err != nil {
				return errors.Trace(err)
			}
		}
		url.ModelQualifier = currentAccountDetails.User
	}
	return nil
}

func (c *revokeCommand) runForOffers(ctx context.Context) error {
	client, err := c.getOfferAPI(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	urls := make([]string, len(c.OfferURLs))
	for i, url := range c.OfferURLs {
		urls[i] = url.String()
	}
	err = client.RevokeOffer(ctx, c.User, c.Access, urls...)
	return block.ProcessBlockedError(err, block.BlockChange)
}
