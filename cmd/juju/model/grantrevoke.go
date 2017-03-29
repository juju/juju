// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"

	"fmt"
	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
)

var usageGrantSummary = `
Grants access level to a Juju user for a model or controller.`[1:]

var usageGrantCmrSummary = `
Grants access level to a Juju user for a model, controller, or application offer.`[1:]

var usageGrantDetails = `
By default, the controller is the current controller.

Users with read access are limited in what they can do with models:
` + "`juju models`, `juju machines`, and `juju status`" + `.

Valid access levels for models are:
    read
    write
    admin

Valid access levels for controllers are:
    login
    add-model
    superuser

Valid access levels for application offers are:
    read
    consume
    admin

Examples:
Grant user 'joe' 'read' access to model 'mymodel':

    juju grant joe read mymodel

Grant user 'jim' 'write' access to model 'mymodel':

    juju grant jim write mymodel

Grant user 'sam' 'read' access to models 'model1' and 'model2':

    juju grant sam read model1 model2

Grant user 'maria' 'add-model' access to the controller:

    juju grant maria add-model
%s
See also: 
    revoke
    add-user`[1:]

var usageGrantCmrDetails = `
Grant user 'joe' 'read' access to application offer 'fred/prod.hosted-mysql':

    juju grant joe read fred/prod.hosted-mysql

Grant user 'jim' 'consume' access to application offer 'fred/prod.hosted-mysql':

    juju grant jim consume fred/prod.hosted-mysql

Grant user 'sam' 'read' access to application offers 'fred/prod.hosted-mysql' and 'mary/test.hosted-mysql':

    juju grant sam read fred/prod.hosted-mysql mary/test.hosted-mysql
`

var usageRevokeSummary = `
Revokes access from a Juju user for a model or controller.`[1:]

var usageRevokeCmrSummary = `
Revokes access from a Juju user for a model, controller, or application offer.`[1:]

var usageRevokeDetails = `
By default, the controller is the current controller.

Revoking write access, from a user who has that permission, will leave
that user with read access. Revoking read access, however, also revokes
write access.

Examples:
Revoke 'read' (and 'write') access from user 'joe' for model 'mymodel':

    juju revoke joe read mymodel

Revoke 'write' access from user 'sam' for models 'model1' and 'model2':

    juju revoke sam write model1 model2

Revoke 'add-model' access from user 'maria' to the controller:

    juju revoke maria add-model
%s
See also: 
    grant`[1:]

var usageRevokeCmrDetails = `
Revoke 'read' (and 'write') access from user 'joe' for application offer 'fred/prod.hosted-mysql':

    juju revoke joe read fred/prod.hosted-mysql

Revoke 'consume' access from user 'sam' for models 'fred/prod.hosted-mysql' and 'mary/test.hosted-mysql':

    juju revoke sam consume fred/prod.hosted-mysql mary/test.hosted-mysql
`

type accessCommand struct {
	modelcmd.ControllerCommandBase

	User       string
	ModelNames []string
	OfferURLs  []*crossmodel.ApplicationURL
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
		if featureflag.Enabled(feature.CrossModelRelations) {
			url, err := crossmodel.ParseApplicationURL(arg)
			if err == nil && url.User == "" {
				details, err := c.ClientStore().AccountDetails(c.ControllerName())
				if err != nil {
					return err
				}
				url.User = details.User
			}
			if err == nil {
				c.OfferURLs = append(c.OfferURLs, url)
				continue
			}
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

	// Special case for backwards compatibility.
	if c.Access == "addmodel" {
		c.Access = "add-model"
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
	cmdArgs := "<user name> <permission> [<model name> ...]"
	cmdSummary := usageGrantSummary
	cmdDoc := usageGrantDetails
	cmrDoc := ""
	if featureflag.Enabled(feature.CrossModelRelations) {
		cmdArgs = "<user name> <permission> [<model name> ... | <offer url> ...]"
		cmrDoc = usageGrantCmrDetails
		cmdSummary = usageGrantCmrSummary
	}
	cmdDoc = fmt.Sprintf(cmdDoc, cmrDoc)

	return &cmd.Info{
		Name:    "grant",
		Args:    cmdArgs,
		Purpose: cmdSummary,
		Doc:     cmdDoc,
	}
}

func (c *grantCommand) getModelAPI() (GrantModelAPI, error) {
	if c.modelsApi != nil {
		return c.modelsApi, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *grantCommand) getControllerAPI() (GrantControllerAPI, error) {
	return c.NewControllerAPIClient()
}

func (c *grantCommand) getOfferAPI(modelName string) (GrantOfferAPI, error) {
	if c.offersApi != nil {
		return c.offersApi, nil
	}
	root, err := c.NewModelAPIRoot(modelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// GrantModelAPI defines the API functions used by the grant command.
type GrantModelAPI interface {
	Close() error
	GrantModel(user, access string, modelUUIDs ...string) error
}

// GrantControllerAPI defines the API functions used by the grant command.
type GrantControllerAPI interface {
	Close() error
	GrantController(user, access string) error
}

// GrantOfferAPI defines the API functions used by the grant command.
type GrantOfferAPI interface {
	Close() error
	GrantOffer(user, access string, offers ...string) error
}

// Run implements cmd.Command.
func (c *grantCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel()
	}
	if len(c.OfferURLs) > 0 {
		return c.runForOffers()
	}
	return c.runForController()
}

func (c *grantCommand) runForController() error {
	client, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.GrantController(c.User, c.Access), block.BlockChange)
}

func (c *grantCommand) runForModel() error {
	client, err := c.getModelAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.GrantModel(c.User, c.Access, models...), block.BlockChange)
}

func (c *grantCommand) runForOffers() error {
	// For each model, process the grants.
	offersForModel := offersForModel(c.OfferURLs)
	for model, urls := range offersForModel {
		client, err := c.getOfferAPI(model)
		if err != nil {
			return err
		}
		defer client.Close()

		err = client.GrantOffer(c.User, c.Access, urls...)
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	return nil
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
	cmdArgs := "<user> <permission> [<model name> ...]"
	cmdSummary := usageRevokeSummary
	cmdDoc := usageRevokeDetails
	cmrDoc := ""
	if featureflag.Enabled(feature.CrossModelRelations) {
		cmdArgs = "<user name> <permission> [<model name> ... | <offer url> ...]"
		cmdSummary = usageRevokeCmrSummary
		cmrDoc = usageRevokeCmrDetails
	}
	cmdDoc = fmt.Sprintf(cmdDoc, cmrDoc)

	return &cmd.Info{
		Name:    "revoke",
		Args:    cmdArgs,
		Purpose: cmdSummary,
		Doc:     cmdDoc,
	}
}

func (c *revokeCommand) getModelAPI() (RevokeModelAPI, error) {
	if c.modelsApi != nil {
		return c.modelsApi, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *revokeCommand) getControllerAPI() (RevokeControllerAPI, error) {
	return c.NewControllerAPIClient()
}

func (c *revokeCommand) getOfferAPI(modelName string) (RevokeOfferAPI, error) {
	if c.offersApi != nil {
		return c.offersApi, nil
	}
	root, err := c.NewModelAPIRoot(modelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationoffers.NewClient(root), nil
}

// RevokeModelAPI defines the API functions used by the revoke command.
type RevokeModelAPI interface {
	Close() error
	RevokeModel(user, access string, modelUUIDs ...string) error
}

// RevokeControllerAPI defines the API functions used by the revoke command.
type RevokeControllerAPI interface {
	Close() error
	RevokeController(user, access string) error
}

// RevokeOfferAPI defines the API functions used by the revoke command.
type RevokeOfferAPI interface {
	Close() error
	RevokeOffer(user, access string, offers ...string) error
}

// Run implements cmd.Command.
func (c *revokeCommand) Run(ctx *cmd.Context) error {
	if len(c.ModelNames) > 0 {
		return c.runForModel()
	}
	if len(c.OfferURLs) > 0 {
		return c.runForOffers()
	}
	return c.runForController()
}

func (c *revokeCommand) runForController() error {
	client, err := c.getControllerAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.RevokeController(c.User, c.Access), block.BlockChange)
}

func (c *revokeCommand) runForModel() error {
	client, err := c.getModelAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := c.ModelUUIDs(c.ModelNames)
	if err != nil {
		return err
	}
	return block.ProcessBlockedError(client.RevokeModel(c.User, c.Access, models...), block.BlockChange)
}

// offersForModel group the offer URLs per model.
func offersForModel(offerURLs []*crossmodel.ApplicationURL) map[string][]string {
	offersForModel := make(map[string][]string)
	for _, url := range offerURLs {
		fullName := jujuclient.JoinOwnerModelName(names.NewUserTag(url.User), url.ModelName)
		offers := offersForModel[fullName]
		offers = append(offers, url.ApplicationName)
		offersForModel[fullName] = offers
	}
	return offersForModel
}

func (c *revokeCommand) runForOffers() error {
	// For each model, process the grant.
	offersForModel := offersForModel(c.OfferURLs)
	for model, urls := range offersForModel {
		client, err := c.getOfferAPI(model)
		if err != nil {
			return err
		}
		defer client.Close()

		err = client.RevokeOffer(c.User, c.Access, urls...)
		if err != nil {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}
	return nil
}
