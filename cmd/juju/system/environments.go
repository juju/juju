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

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
)

// EnvironmentsCommand returns the list of all the environments the
// current user can access on the current system.
type EnvironmentsCommand struct {
	envcmd.SysCommandBase
	out       cmd.Output
	all       bool
	user      string
	listUUID  bool
	exactTime bool
	envAPI    EnvironmentsEnvAPI
	sysAPI    EnvironmentsSysAPI
	userCreds *configstore.APICredentials
}

var envsDoc = `
List all the environments the user can access on the current system.

The environments listed here are either environments you have created
yourself, or environments which have been shared with you.

See Also:
    juju help juju-systems
    juju help systems
    juju help environment users
    juju help environment share
    juju help environment unshare
`

// EnvironmentsEnvAPI defines the methods on the environment manager API that
// the environments command calls.
type EnvironmentsEnvAPI interface {
	Close() error
	ListEnvironments(user string) ([]base.UserEnvironment, error)
}

// EnvironmentsSysAPI defines the methods on the system manager API that the
// environments command calls.
type EnvironmentsSysAPI interface {
	Close() error
	AllEnvironments() ([]base.UserEnvironment, error)
}

// Info implements Command.Info
func (c *EnvironmentsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "environments",
		Purpose: "list all environments the user can access on the current system",
		Doc:     envsDoc,
	}
}

func (c *EnvironmentsCommand) getEnvAPI() (EnvironmentsEnvAPI, error) {
	if c.envAPI != nil {
		return c.envAPI, nil
	}
	return c.NewEnvironmentManagerAPIClient()
}

func (c *EnvironmentsCommand) getSysAPI() (EnvironmentsSysAPI, error) {
	if c.sysAPI != nil {
		return c.sysAPI, nil
	}
	return c.NewSystemManagerAPIClient()
}

func (c *EnvironmentsCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	if c.userCreds != nil {
		return *c.userCreds, nil
	}
	return c.ConnectionCredentials()
}

// SetFlags implements Command.SetFlags.
func (c *EnvironmentsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "", "the user to list environments for (administrative users only)")
	f.BoolVar(&c.all, "all", false, "show all environments  (administrative users only)")
	f.BoolVar(&c.listUUID, "uuid", false, "display UUID for environments")
	f.BoolVar(&c.exactTime, "exact-time", false, "use full timestamp precision")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Local structure that controls the output structure.
type UserEnvironment struct {
	Name           string `json:"name"`
	UUID           string `json:"env-uuid" yaml:"env-uuid"`
	Owner          string `json:"owner"`
	LastConnection string `json:"last-connection" yaml:"last-connection"`
}

// Run implements Command.Run
func (c *EnvironmentsCommand) Run(ctx *cmd.Context) error {
	if c.user == "" {
		creds, err := c.getConnectionCredentials()
		if err != nil {
			return errors.Trace(err)
		}
		c.user = creds.User
	}

	var envs []base.UserEnvironment
	var err error
	if c.all {
		envs, err = c.getAllEnvironments()
	} else {
		envs, err = c.getUserEnvironments()
	}
	if err != nil {
		return errors.Annotate(err, "cannot list environments")
	}

	output := make([]UserEnvironment, len(envs))
	now := time.Now()
	for i, env := range envs {
		output[i] = UserEnvironment{
			Name:           env.Name,
			UUID:           env.UUID,
			Owner:          env.Owner,
			LastConnection: user.LastConnection(env.LastConnection, now, c.exactTime),
		}
	}

	return c.out.Write(ctx, output)
}

func (c *EnvironmentsCommand) getAllEnvironments() ([]base.UserEnvironment, error) {
	client, err := c.getSysAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.AllEnvironments()
}

func (c *EnvironmentsCommand) getUserEnvironments() ([]base.UserEnvironment, error) {
	client, err := c.getEnvAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.ListEnvironments(c.user)
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *EnvironmentsCommand) formatTabular(value interface{}) ([]byte, error) {
	envs, ok := value.([]UserEnvironment)
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
		fmt.Fprintf(tw, "\t%s\t%s\n", env.Owner, env.LastConnection)
	}
	tw.Flush()
	return out.Bytes(), nil
}
