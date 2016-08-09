// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
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
		Aliases: []string{"show-controllers"},
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
	for _, name := range controllerNames {
		one, err := c.store.ControllerByName(name)
		if err != nil {
			return err
		}
		controllers[name] = c.convertControllerForShow(name, one)
	}
	return c.out.Write(ctx, controllers)
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

	// Password is the password for the account.
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

func (c *showControllerCommand) convertControllerForShow(controllerName string, details *jujuclient.ControllerDetails) ShowControllerDetails {
	controller := ShowControllerDetails{
		Details: ControllerDetails{
			ControllerUUID: details.ControllerUUID,
			APIEndpoints:   details.APIEndpoints,
			CACert:         details.CACert,
			Cloud:          details.Cloud,
			CloudRegion:    details.CloudRegion,
		},
	}
	c.convertModelsForShow(controllerName, &controller)
	c.convertAccountsForShow(controllerName, &controller)
	return controller
}

func (c *showControllerCommand) convertAccountsForShow(controllerName string, controller *ShowControllerDetails) {
	storeDetails, err := c.store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
	if storeDetails == nil {
		return
	}
	details := &AccountDetails{
		User: storeDetails.User,
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

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore

	controllerNames []string
	showPasswords   bool
}
