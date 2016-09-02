// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"bytes"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

var usageGetConfigSummary = `
Displays configuration settings for a deployed application.`[1:]

var usageGetConfigDetails = `
By default, all configuration (keys, values, metadata) for the application are
displayed if a key is not specified.
Output includes the name of the charm used to deploy the application and a
listing of the application-specific configuration settings.
See `[1:] + "`juju status`" + ` for application names.

Examples:
    juju get-config mysql
    juju get-config mysql-testing
    juju get-config mysql wait-timeout

See also: 
    set-config
    deploy
    status`

// NewGetCommand returns a command used to get application attributes.
func NewGetCommand() cmd.Command {
	return modelcmd.Wrap(&getCommand{})
}

// getCommand retrieves the configuration of an application.
type getCommand struct {
	modelcmd.ModelCommandBase
	applicationName string
	key             string
	out             cmd.Output
	api             getServiceAPI
}

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-config",
		Args:    "<application name> [attribute-key]",
		Purpose: usageGetConfigSummary,
		Doc:     usageGetConfigDetails,
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

func (c *getCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.applicationName = args[0]
	if len(args) == 1 {
		return nil
	}
	c.key = args[1]
	return cmd.CheckEmpty(args[2:])
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

	results, err := apiclient.Get(c.applicationName)
	if err != nil {
		return err
	}
	if c.key != "" {
		info, found := results.Config[c.key].(map[string]interface{})
		if !found {
			return errors.Errorf("key %q not found in %q application settings.", c.key, c.applicationName)
		}
		out := &bytes.Buffer{}
		err := cmd.FormatYaml(out, info["value"])
		if err != nil {
			return err
		}
		fmt.Fprint(ctx.Stdout, out.String())
		return nil
	}

	resultsMap := map[string]interface{}{
		"application": results.Application,
		"charm":       results.Charm,
		"settings":    results.Config,
	}
	return c.out.Write(ctx, resultsMap)
}
