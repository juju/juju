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
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
)

// NewEnvironmentsCommand returns a command to list environments.
func NewEnvironmentsCommand() cmd.Command {
	return envcmd.WrapController(&environmentsCommand{})
}

// environmentsCommand returns the list of all the environments the
// current user can access on the current controller.
type environmentsCommand struct {
	envcmd.ControllerCommandBase
	out       cmd.Output
	all       bool
	user      string
	listUUID  bool
	exactTime bool
	modelAPI  ModelManagerAPI
	sysAPI    ModelsSysAPI
	userCreds *configstore.APICredentials
}

var envsDoc = `
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
// environments command calls.
type ModelsSysAPI interface {
	Close() error
	AllModels() ([]base.UserModel, error)
}

// Info implements Command.Info
func (c *environmentsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-models",
		Purpose: "list all models the user can access on the current controller",
		Doc:     envsDoc,
	}
}

func (c *environmentsCommand) getEnvAPI() (ModelManagerAPI, error) {
	if c.modelAPI != nil {
		return c.modelAPI, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *environmentsCommand) getSysAPI() (ModelsSysAPI, error) {
	if c.sysAPI != nil {
		return c.sysAPI, nil
	}
	return c.NewControllerAPIClient()
}

func (c *environmentsCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	if c.userCreds != nil {
		return *c.userCreds, nil
	}
	return c.ConnectionCredentials()
}

// SetFlags implements Command.SetFlags.
func (c *environmentsCommand) SetFlags(f *gnuflag.FlagSet) {
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
	UUID           string `json:"env-uuid" yaml:"env-uuid"`
	Owner          string `json:"owner"`
	LastConnection string `json:"last-connection" yaml:"last-connection"`
}

// Run implements Command.Run
func (c *environmentsCommand) Run(ctx *cmd.Context) error {
	if c.user == "" {
		creds, err := c.getConnectionCredentials()
		if err != nil {
			return errors.Trace(err)
		}
		c.user = creds.User
	}

	var envs []base.UserModel
	var err error
	if c.all {
		envs, err = c.getAllModels()
	} else {
		envs, err = c.getUserModels()
	}
	if err != nil {
		return errors.Annotate(err, "cannot list models")
	}

	output := make([]UserModel, len(envs))
	now := time.Now()
	for i, env := range envs {
		output[i] = UserModel{
			Name:           env.Name,
			UUID:           env.UUID,
			Owner:          env.Owner,
			LastConnection: user.LastConnection(env.LastConnection, now, c.exactTime),
		}
	}

	return c.out.Write(ctx, output)
}

func (c *environmentsCommand) getAllModels() ([]base.UserModel, error) {
	client, err := c.getSysAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.AllModels()
}

func (c *environmentsCommand) getUserModels() ([]base.UserModel, error) {
	client, err := c.getEnvAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.ListModels(c.user)
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *environmentsCommand) formatTabular(value interface{}) ([]byte, error) {
	envs, ok := value.([]UserModel)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", envs, value)
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
	for _, env := range envs {
		fmt.Fprintf(tw, "%s", env.Name)
		if c.listUUID {
			fmt.Fprintf(tw, "\t%s", env.UUID)
		}
		fmt.Fprintf(tw, "\t%s\t%s\n", env.Owner, env.LastConnection)
	}
	tw.Flush()
	return out.Bytes(), nil
}
