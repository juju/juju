// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
)

// EnvironmentsCommand returns the list of all the environments the
// current user can access on the current system.
type EnvironmentsCommand struct {
	envcmd.SysCommandBase
	out       cmd.Output
	user      string
	listUUID  bool
	api       EnvironmentManagerAPI
	userCreds *configstore.APICredentials
}

var envsDoc = `List all the environments the user can access on the current system`

// EnvironmentManagerAPI defines the methods on the client API that the
// environments command calls.
type EnvironmentManagerAPI interface {
	Close() error
	ListEnvironments(user string) ([]params.UserEnvironment, error)
}

// Info implements Command.Info
func (c *EnvironmentsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "environments",
		Purpose: "list all environments the user can access on the current system",
		Doc:     envsDoc,
	}
}

func (c *EnvironmentsCommand) getAPI() (EnvironmentManagerAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewEnvironmentManagerAPIClient()
}

func (c *EnvironmentsCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	if c.userCreds != nil {
		return *c.userCreds, nil
	}
	return c.ConnectionCredentials()
}

// SetFlags implements Command.SetFlags.
func (c *EnvironmentsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "", "the user to list environments for.  Only administrative users can list environments for other users.")
	f.BoolVar(&c.listUUID, "uuid", false, "Display UUID for environments")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Run implements Command.Run
func (c *EnvironmentsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if c.user == "" {
		creds, err := c.getConnectionCredentials()
		if err != nil {
			return errors.Trace(err)
		}
		c.user = creds.User
	}

	envs, err := client.ListEnvironments(c.user)
	if err != nil {
		return errors.Annotate(err, "cannot list environments")
	}

	return c.out.Write(ctx, envs)
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *EnvironmentsCommand) formatTabular(value interface{}) ([]byte, error) {
	envs, ok := value.([]params.UserEnvironment)
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
		fmt.Fprintf(tw, "\tENVIRONMENT UUID")
	}
	fmt.Fprintf(tw, "\tOWNER\tLAST CONNECTION\n")
	for _, env := range envs {
		fmt.Fprintf(tw, "%s", env.Name)
		if c.listUUID {
			fmt.Fprintf(tw, "\t%s", env.UUID)
		}
		lastConn := "never connected"
		if env.LastConnection != nil {
			lastConn = user.UserFriendlyDuration(*env.LastConnection, time.Now())
		}
		fmt.Fprintf(tw, "\t%s\t%s\n", env.OwnerTag, lastConn)
	}
	tw.Flush()
	return out.Bytes(), nil
}
