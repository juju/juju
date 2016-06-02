// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

var usageShowControllerSummary = `
Shows detailed information of a controller.`[1:]

var usageShowControllerDetails = `
Shows extended information about a controller(s) as well as related models
and accounts. The active model and user accounts are also displayed.

Examples:
    juju show-controller
    juju show-controller aws google
    
See also: 
    list-controllers`[1:]

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
	f.BoolVar(&c.showPasswords, "show-passwords", false, "Show passwords for displayed accounts")
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

	// Accounts is a collection of accounts for this controller.
	Accounts map[string]*AccountDetails `yaml:"accounts,omitempty" json:"accounts,omitempty"`

	// CurrentAccount is the name of the current account for this controller.
	CurrentAccount string `yaml:"current-account,omitempty" json:"current-account,omitempty"`

	// BootstrapConfig contains the bootstrap configuration for this controller.
	// This is only available on the client that bootstrapped the controller.
	BootstrapConfig *BootstrapConfig `yaml:"bootstrap-config,omitempty" json:"bootstrap-config,omitempty"`

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

	// Models is a collection of all models for this controller.
	Models map[string]ModelDetails `yaml:"models,omitempty" json:"models,omitempty"`

	// CurrentModel is the name of the current model for this controller
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`
}

// BootstrapConfig holds the configuration used to bootstrap a controller.
type BootstrapConfig struct {
	Config               map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	Cloud                string                 `yaml:"cloud" json:"cloud"`
	CloudType            string                 `yaml:"cloud-type" json:"cloud-type"`
	CloudRegion          string                 `yaml:"region,omitempty" json:"region,omitempty"`
	CloudEndpoint        string                 `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	CloudStorageEndpoint string                 `yaml:"storage-endpoint,omitempty" json:"storage-endpoint,omitempty"`
	Credential           string                 `yaml:"credential,omitempty" json:"credential,omitempty"`
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
	c.convertAccountsForShow(controllerName, &controller)
	c.convertBootstrapConfigForShow(controllerName, &controller)
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

func (c *showControllerCommand) convertBootstrapConfigForShow(controllerName string, controller *ShowControllerDetails) {
	bootstrapConfig, err := c.store.BootstrapConfigForController(controllerName)
	if errors.IsNotFound(err) {
		return
	} else if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
		return
	}
	cfg := make(map[string]interface{})
	var cloudType string
	for k, v := range bootstrapConfig.Config {
		switch k {
		case config.NameKey:
			// Name is always "admin" for the admin model,
			// which is not interesting to us here.
		case config.TypeKey:
			// Pull Type up to the top level.
			cloudType = fmt.Sprint(v)
		default:
			cfg[k] = v
		}
	}
	controller.BootstrapConfig = &BootstrapConfig{
		Config:               cfg,
		Cloud:                bootstrapConfig.Cloud,
		CloudType:            cloudType,
		CloudRegion:          bootstrapConfig.CloudRegion,
		CloudEndpoint:        bootstrapConfig.CloudEndpoint,
		CloudStorageEndpoint: bootstrapConfig.CloudStorageEndpoint,
		Credential:           bootstrapConfig.Credential,
	}
}

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore

	controllerNames []string
	showPasswords   bool
}
