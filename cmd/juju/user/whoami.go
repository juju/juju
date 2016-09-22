// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
)

var whoAmIDetails = `
Display the current controller, model and logged in user name. 

Examples:
    juju whoami

See also:
    controllers
    login
    logout
    models
    users
`[1:]

// NewWhoAmICommand returns a command to print login details.
func NewWhoAmICommand() cmd.Command {
	cmd := &whoAmICommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Info implements Command.Info
func (c *whoAmICommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "whoami",
		Purpose: "Print current login details",
		Doc:     whoAmIDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *whoAmICommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatWhoAmITabular,
	})
}

type whoAmI struct {
	ControllerName string `yaml:"controller" json:"controller"`
	ModelName      string `yaml:"model,omitempty" json:"model,omitempty"`
	UserName       string `yaml:"user" json:"user"`
}

func formatWhoAmITabular(writer io.Writer, value interface{}) error {
	details, ok := value.(whoAmI)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", details, value)
	}
	tw := output.TabWriter(writer)
	fmt.Fprintf(tw, "Controller:\t%s\n", details.ControllerName)
	modelName := details.ModelName
	if modelName == "" {
		modelName = "<no-current-model>"
	}
	fmt.Fprintf(tw, "Model:\t%s\n", modelName)
	fmt.Fprintf(tw, "User:\t%s", details.UserName)
	return tw.Flush()
}

// Run implements Command.Run
func (c *whoAmICommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.store.CurrentController()
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err != nil {
		fmt.Fprintln(ctx.Stderr, "There is no current controller.\nRun juju list-controllers to see available controllers.")
		return nil
	}
	modelName, err := c.store.CurrentModel(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	userDetails, err := c.store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "You are not logged in to controller %q and model %q.\nRun juju login if you want to login.\n", controllerName, modelName)
		return nil
	}
	// Only qualify model name if there is a current model.
	if modelName != "" {
		if unqualifiedModelName, owner, err := jujuclient.SplitModelName(modelName); err == nil {
			user := names.NewUserTag(userDetails.User)
			modelName = common.OwnerQualifiedModelName(unqualifiedModelName, owner, user)
		}
	}

	result := whoAmI{
		ControllerName: controllerName,
		ModelName:      modelName,
		UserName:       userDetails.User,
	}
	return c.out.Write(ctx, result)
}

type whoAmICommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore
}
