// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
)

// NewListModelsCommand returns a command to list models.
func NewListModelsCommand() cmd.Command {
	return modelcmd.WrapController(&modelsCommand{})
}

// modelsCommand returns the list of all the models the
// current user can access on the current controller.
type modelsCommand struct {
	modelcmd.ControllerCommandBase
	out       cmd.Output
	all       bool
	user      string
	listUUID  bool
	exactTime bool
	modelAPI  ModelManagerAPI
	sysAPI    ModelsSysAPI
	userCreds *configstore.APICredentials
}

var listModelsDoc = `
List all the models the user can access on the current controller.

The models listed here are either models you have created
yourself, or models which have been shared with you.

See Also:
    juju help controllers
    juju help model users
    juju help model share
    juju help model unshare
`

// ModelManagerAPI defines the methods on the model manager API that
// the models command calls.
type ModelManagerAPI interface {
	Close() error
	ListModels(user string) ([]base.UserModel, error)
}

// ModelsSysAPI defines the methods on the controller manager API that the
// list models command calls.
type ModelsSysAPI interface {
	Close() error
	AllModels() ([]base.UserModel, error)
}

// Info implements Command.Info
func (c *modelsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-models",
		Purpose: "list all models the user can access on the current controller",
		Doc:     listModelsDoc,
	}
}

func (c *modelsCommand) getModelManagerAPI() (ModelManagerAPI, error) {
	if c.modelAPI != nil {
		return c.modelAPI, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *modelsCommand) getSysAPI() (ModelsSysAPI, error) {
	if c.sysAPI != nil {
		return c.sysAPI, nil
	}
	return c.NewControllerAPIClient()
}

func (c *modelsCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	if c.userCreds != nil {
		return *c.userCreds, nil
	}
	return c.ConnectionCredentials()
}

// SetFlags implements Command.SetFlags.
func (c *modelsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "", "the user to list models for (administrative users only)")
	f.BoolVar(&c.all, "all", false, "show all models  (administrative users only)")
	f.BoolVar(&c.listUUID, "uuid", false, "display UUID for models")
	f.BoolVar(&c.exactTime, "exact-time", false, "use full timestamp precision")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Local structure that controls the output structure.
type UserModel struct {
	Name           string `json:"name"`
	UUID           string `json:"model-uuid" yaml:"model-uuid"`
	Owner          string `json:"owner"`
	LastConnection string `json:"last-connection" yaml:"last-connection"`
}

// Run implements Command.Run
func (c *modelsCommand) Run(ctx *cmd.Context) error {
	if c.user == "" {
		creds, err := c.getConnectionCredentials()
		if err != nil {
			return errors.Trace(err)
		}
		c.user = creds.User
	}

	var models []base.UserModel
	var err error
	if c.all {
		models, err = c.getAllModels()
	} else {
		models, err = c.getUserModels()
	}
	if err != nil {
		return errors.Annotate(err, "cannot list models")
	}

	output := make([]UserModel, len(models))
	now := time.Now()
	for i, model := range models {
		output[i] = UserModel{
			Name:           model.Name,
			UUID:           model.UUID,
			Owner:          model.Owner,
			LastConnection: user.LastConnection(model.LastConnection, now, c.exactTime),
		}
	}

	return c.out.Write(ctx, output)
}

func (c *modelsCommand) getAllModels() ([]base.UserModel, error) {
	client, err := c.getSysAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.AllModels()
}

func (c *modelsCommand) getUserModels() ([]base.UserModel, error) {
	client, err := c.getModelManagerAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.ListModels(c.user)
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *modelsCommand) formatTabular(value interface{}) ([]byte, error) {
	models, ok := value.([]UserModel)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", models, value)
	}
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME")
	if c.listUUID {
		fmt.Fprintf(tw, "\tMODEL UUID")
	}
	fmt.Fprintf(tw, "\tOWNER\tLAST CONNECTION\n")
	for _, model := range models {
		fmt.Fprintf(tw, "%s", model.Name)
		if c.listUUID {
			fmt.Fprintf(tw, "\t%s", model.UUID)
		}
		fmt.Fprintf(tw, "\t%s\t%s\n", model.Owner, model.LastConnection)
	}
	tw.Flush()
	return out.Bytes(), nil
}
