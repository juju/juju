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
	if len(args) < 1 {
		return errors.New("must specify controller name(s)")
	}
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
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run
func (c *showControllerCommand) Run(ctx *cmd.Context) error {
	controllers := make(map[string]ShowControllerDetails)
	for _, name := range c.controllerNames {
		one, err := c.store.ControllerByName(name)
		if err != nil {
			return errors.Annotatef(err, "failed to get controller %s", name)
		}
		controllers[name] = c.convertControllerForShow(name, one)
	}
	return c.out.Write(ctx, controllers)
}

func (c *showControllerCommand) convertControllerForShow(controllerName string, details *jujuclient.ControllerDetails) ShowControllerDetails {
	controller := ShowControllerDetails{Details: details}

	var err error
	controller.Models, err = c.store.AllModels(controllerName)
	if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
	}
	controller.CurrentModel, err = c.store.CurrentModel(controllerName)
	if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
	}

	controller.Accounts, err = c.store.AllAccounts(controllerName)
	if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
	}
	controller.CurrentAccount, err = c.store.CurrentAccount(controllerName)
	if err != nil {
		controller.Errors = append(controller.Errors, err.Error())
	}
	return controller
}

type ShowControllerDetails struct {
	// Details contains the same details that client store caches for this controller.
	Details *jujuclient.ControllerDetails `yaml:"details,omitempty" json:"details,omitempty"`

	// Models is a collection of all models for this controller.
	Models map[string]jujuclient.ModelDetails `yaml:"models,omitempty" json:"models,omitempty"`

	// CurrentModel is the name of the current model for this controller
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`

	// Accounts is a collection of accounts for this controller.
	Accounts map[string]jujuclient.AccountDetails `yaml:"accounts,omitempty" json:"accounts,omitempty"`

	// CurrentAccount is the name of the current account for this controller.
	CurrentAccount string `yaml:"current-account,omitempty" json:"current-account,omitempty"`

	// Errors is a collection of errors related to accessing this controller details.
	Errors []string `yaml:"errors,omitempty" json:"errors,omitempty"`
}

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out             cmd.Output
	store           jujuclient.ClientStore
	controllerNames []string
}

const showControllerDoc = `
Show extended information about controller(s) as well as related models and accounts.
Both current and last used model and account are displayed as well.

Controllers to display are specified by controller names.

arguments:
<space separated controller names>
`
