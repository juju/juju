// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/jujuclient"
)

var usageShowControllerSummary = `
Shows detailed information of a controller.`[1:]

var usageShowControllerDetails = `
Shows extended information about a controller(s) as well as related models
and user login details.

Examples:
    juju show-controller
    juju show-controller aws google
    
See also: 
    controllers`[1:]

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore
	api   controllerAccessAPI

	controllerNames []string
	showPasswords   bool
}

// NewShowControllerCommand returns a command to show details of the desired controllers.
func NewShowControllerCommand() cmd.Command {
	cmd := &showControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Init implements Command.Init.
func (c *showControllerCommand) Init(args []string) (err error) {
	c.controllerNames = args
	return nil
}

// Info implements Command.Info
func (c *showControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-controller",
		Args:    "[<controller name> ...]",
		Purpose: usageShowControllerSummary,
		Doc:     usageShowControllerDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	f.BoolVar(&c.showPasswords, "show-password", false, "Show password for logged in user")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// ControllerAccessAPI defines a subset of the api/controller/Client API.
type controllerAccessAPI interface {
	GetControllerAccess(user string) (description.Access, error)
	ModelConfig() (map[string]interface{}, error)
	Close() error
}

func (c *showControllerCommand) getAPI(controllerName string) (controllerAccessAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot(c.store, controllerName, "")
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return controller.NewClient(api), nil
}

// Run implements Command.Run
func (c *showControllerCommand) Run(ctx *cmd.Context) error {
	controllerNames := c.controllerNames
	if len(controllerNames) == 0 {
		currentController, err := c.store.CurrentController()
		if errors.IsNotFound(err) {
			return errors.New("there is no active controller")
		} else if err != nil {
			return errors.Trace(err)
		}
		controllerNames = []string{currentController}
	}
	controllers := make(map[string]ShowControllerDetails)
	for _, controllerName := range controllerNames {
		one, err := c.store.ControllerByName(controllerName)
		if err != nil {
			return err
		}
		var access string
		accountDetails, err := c.store.AccountDetails(controllerName)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, err)
			access = "(error)"
		} else {
			client, err := c.getAPI(controllerName)
			if err != nil {
				return err
			}
			defer client.Close()
			access = c.userAccess(client, ctx, accountDetails.User)
			one.AgentVersion = c.agentVersion(client, ctx)
		}
		controllers[controllerName] = c.convertControllerForShow(controllerName, one, access)
	}
	return c.out.Write(ctx, controllers)
}

func (c *showControllerCommand) userAccess(client controllerAccessAPI, ctx *cmd.Context, user string) string {
	var access string
	userAccess, err := client.GetControllerAccess(user)
	if err == nil {
		access = string(userAccess)
	} else {
		code := params.ErrCode(err)
		if code != "" {
			access = fmt.Sprintf("(%s)", code)
		} else {
			fmt.Fprintln(ctx.Stderr, err)
			access = "(error)"
		}
	}
	return access
}

func (c *showControllerCommand) agentVersion(client controllerAccessAPI, ctx *cmd.Context) string {
	var ver string
	mc, err := client.ModelConfig()
	if err != nil {
		code := params.ErrCode(err)
		if code != "" {
			ver = fmt.Sprintf("(%s)", code)
		} else {
			fmt.Fprintln(ctx.Stderr, err)
			ver = "(error)"
		}
		return ver
	}
	return mc["agent-version"].(string)
}

type ShowControllerDetails struct {
	// Details contains the same details that client store caches for this controller.
	Details ControllerDetails `yaml:"details,omitempty" json:"details,omitempty"`

	// Models is a collection of all models for this controller.
	Models map[string]ModelDetails `yaml:"models,omitempty" json:"models,omitempty"`

	// CurrentModel is the name of the current model for this controller
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`

	// Account is the account details for the user logged into this controller.
	Account *AccountDetails `yaml:"account,omitempty" json:"account,omitempty"`

	// Errors is a collection of errors related to accessing this controller details.
	Errors []string `yaml:"errors,omitempty" json:"errors,omitempty"`
}

// ControllerDetails holds details of a controller to show.
type ControllerDetails struct {
	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid" json:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow" json:"api-endpoints"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert" json:"ca-cert"`

	// Cloud is the name of the cloud that this controller runs in.
	Cloud string `yaml:"cloud" json:"cloud"`

	// CloudRegion is the name of the cloud region that this controller runs in.
	CloudRegion string `yaml:"region,omitempty" json:"region,omitempty"`

	// AgentVersion is the version of the agent running on this controller.
	// AgentVersion need not always exist so we omitempty here. This struct is
	// used in both list-controller and show-controller. show-controller
	// displays the agent version where list-controller does not.
	AgentVersion string `yaml:"agent-version,omitempty" json:"agent-version,omitempty"`
}

// ModelDetails holds details of a model to show.
type ModelDetails struct {
	// ModelUUID holds the details of a model.
	ModelUUID string `yaml:"uuid" json:"uuid"`
}

// AccountDetails holds details of an account to show.
type AccountDetails struct {
	// User is the username for the account.
	User string `yaml:"user" json:"user"`

	// Access is the level of access the user has on the controller.
	Access string `yaml:"access,omitempty" json:"access,omitempty"`

	// Password is the password for the account.
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

func (c *showControllerCommand) convertControllerForShow(controllerName string, details *jujuclient.ControllerDetails, access string) ShowControllerDetails {
	controller := ShowControllerDetails{
		Details: ControllerDetails{
			ControllerUUID: details.ControllerUUID,
			APIEndpoints:   details.APIEndpoints,
			CACert:         details.CACert,
			Cloud:          details.Cloud,
			CloudRegion:    details.CloudRegion,
			AgentVersion:   details.AgentVersion,
		},
	}
	c.convertModelsForShow(controllerName, &controller)
	c.convertAccountsForShow(controllerName, &controller, access)
	return controller
}

func (c *showControllerCommand) convertAccountsForShow(controllerName string, controller *ShowControllerDetails, access string) {
	storeDetails, err := c.store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
	if storeDetails == nil {
		return
	}
	details := &AccountDetails{
		User:   storeDetails.User,
		Access: access,
	}
	if c.showPasswords {
		details.Password = storeDetails.Password
	}
	controller.Account = details
}

func (c *showControllerCommand) convertModelsForShow(controllerName string, controller *ShowControllerDetails) {
	models, err := c.store.AllModels(controllerName)
	if errors.IsNotFound(err) {
		return
	} else if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
		return
	}
	if len(models) > 0 {
		controller.Models = make(map[string]ModelDetails)
		for modelName, model := range models {
			controller.Models[modelName] = ModelDetails{model.ModelUUID}
		}
	}
	controller.CurrentModel, err = c.store.CurrentModel(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
		return
	}
}
