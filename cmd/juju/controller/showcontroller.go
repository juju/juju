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
		Purpose: "show controller details for the given controller names",
		Doc:     showControllerDoc,
		Aliases: []string{"show-controllers"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	f.BoolVar(&c.showPasswords, "show-passwords", false, "show passwords for displayed accounts")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run
func (c *showControllerCommand) Run(ctx *cmd.Context) error {
	controllerNames := c.controllerNames
	if len(controllerNames) == 0 {
		currentController, err := modelcmd.ReadCurrentController()
		if err != nil {
			return errors.Trace(err)
		}
		if currentController == "" {
			return errors.New("there is no active controller")
		}
		controllerNames = []string{currentController}
	}
	controllers := make(map[string]ShowControllerDetails)
	for _, name := range controllerNames {
		actualName, err := modelcmd.ResolveControllerName(c.store, name)
		if err != nil {
			return err
		}
		one, err := c.store.ControllerByName(actualName)
		if err != nil {
			return err
		}
		controllers[name] = c.convertControllerForShow(actualName, one)
	}
	return c.out.Write(ctx, controllers)
}

type ShowControllerDetails struct {
	// Details contains the same details that client store caches for this controller.
	Details ControllerDetails `yaml:"details,omitempty" json:"details,omitempty"`

	// Accounts is a collection of accounts for this controller.
	Accounts map[string]*AccountDetails `yaml:"accounts,omitempty" json:"accounts,omitempty"`

	// CurrentAccount is the name of the current account for this controller.
	CurrentAccount string `yaml:"current-account,omitempty" json:"current-account,omitempty"`

	// Errors is a collection of errors related to accessing this controller details.
	Errors []string `yaml:"errors,omitempty" json:"errors,omitempty"`
}

// ControllerDetails holds details of a controller to show.
type ControllerDetails struct {
	// Servers contains the addresses of hosts that form this controller's cluster.
	Servers []string `yaml:"servers,flow" json:"servers"`

	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid" json:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow" json:"api-endpoints"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert" json:"ca-cert"`
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

	// Models is a collection of all models for this controller.
	Models map[string]ModelDetails `yaml:"models,omitempty" json:"models,omitempty"`

	// CurrentModel is the name of the current model for this controller
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`
}

func (c *showControllerCommand) convertControllerForShow(controllerName string, details *jujuclient.ControllerDetails) ShowControllerDetails {
	controller := ShowControllerDetails{
		Details: ControllerDetails{
			Servers:        details.Servers,
			ControllerUUID: details.ControllerUUID,
			APIEndpoints:   details.APIEndpoints,
			CACert:         details.CACert,
		},
	}

	c.convertAccountsForShow(controllerName, &controller)
	return controller
}

func (c *showControllerCommand) convertAccountsForShow(controllerName string, controller *ShowControllerDetails) {
	accounts, err := c.store.AllAccounts(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}

	if len(accounts) > 0 {
		controller.Accounts = make(map[string]*AccountDetails)
		for accountName, account := range accounts {
			details := &AccountDetails{User: account.User}
			controller.Accounts[accountName] = details
			if c.showPasswords {
				details.Password = account.Password
			}
			if err := c.convertModelsForShow(controllerName, accountName, details); err != nil {
				controller.Errors = append(controller.Errors, err.Error())
			}
		}
	}

	controller.CurrentAccount, err = c.store.CurrentAccount(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
}

func (c *showControllerCommand) convertModelsForShow(controllerName, accountName string, account *AccountDetails) error {
	models, err := c.store.AllModels(controllerName, accountName)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	if len(models) > 0 {
		account.Models = make(map[string]ModelDetails)
		for modelName, model := range models {
			account.Models[modelName] = ModelDetails{model.ModelUUID}
		}
	}
	account.CurrentModel, err = c.store.CurrentModel(controllerName, accountName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore

	controllerNames []string
	showPasswords   bool
}

const showControllerDoc = `
Show extended information about controller(s) as well as related models and accounts.
The active model and account for each controller are displayed as well.

Controllers to display are specified by controller names. If no controller names
are supplied, show-controller will print out the active controller.

arguments:
[space separated controller names]
`
