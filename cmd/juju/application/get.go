// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageGetConfigSummary = `
Displays configuration settings for a deployed application.`[1:]

var usageGetConfigDetails = `
Output includes the name of the charm used to deploy the application and a
listing of the application-specific configuration settings.
See `[1:] + "`juju status`" + ` for application names.

Examples:
    juju get-config mysql
    juju get-config mysql-testing

See also: 
    set-config
    deploy
    status`

// NewGetCommand returns a command used to get application attributes.
func NewGetCommand() cmd.Command {
	return modelcmd.Wrap(&getCommand{})
}

// getCommand retrieves the configuration of a application.
type getCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
	out             cmd.Output
	api             getServiceAPI
}

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-config",
		Args:    "<application name>",
		Purpose: usageGetConfigSummary,
		Doc:     usageGetConfigDetails,
		Aliases: []string{"get-configs"},
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO(dfc) add json formatting ?
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

func (c *getCommand) Init(args []string) error {
	// TODO(dfc) add --schema-only
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.ApplicationName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// getServiceAPI defines the methods on the client API
// that the application get command calls.
type getServiceAPI interface {
	Close() error
	Get(application string) (*params.ApplicationGetResults, error)
}

func (c *getCommand) getAPI() (getServiceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Run fetches the configuration of the application and formats
// the result as a YAML string.
func (c *getCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	results, err := apiclient.Get(c.ApplicationName)
	if err != nil {
		return err
	}

	resultsMap := map[string]interface{}{
		"application": results.Application,
		"charm":       results.Charm,
		"settings":    results.Config,
	}
	return c.out.Write(ctx, resultsMap)
}
