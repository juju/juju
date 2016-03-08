// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewGetCommand returns a command used to get service attributes.
func NewGetCommand() cmd.Command {
	return modelcmd.Wrap(&getCommand{})
}

// getCommand retrieves the configuration of a service.
type getCommand struct {
	modelcmd.ModelCommandBase
	ServiceName string
	out         cmd.Output
	api         getServiceAPI
}

const getDoc = `
The command output includes the service and charm names, a detailed list of all config
settings for <service>, including the setting name, whether it uses the default value
or not ("default: true"), description (if set), type, and current value. Example:

$ juju get-config wordpress

charm: wordpress
service: wordpress
settings:
  engine:
      default: true
      description: 'Currently two ...'
      type: string
      value: nginx
   tuning:
      description: "This is the tuning level..."
      type: string
      value: optimized

NOTE: In the example above the descriptions and most other settings were omitted or
truncated for brevity. The "engine" setting was left at its default value ("nginx"),
while the "tuning" setting was set to "optimized" (the default value is "single").

Note that the "default" field indicates whether a configuration setting is at its
default value. It does not indicate the default value for the setting.
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-config",
		Args:    "<service>",
		Purpose: "get service configuration options",
		Doc:     getDoc,
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
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// getServiceAPI defines the methods on the client API
// that the service get command calls.
type getServiceAPI interface {
	Close() error
	Get(service string) (*params.ServiceGetResults, error)
}

func (c *getCommand) getAPI() (getServiceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service.NewClient(root), nil
}

// Run fetches the configuration of the service and formats
// the result as a YAML string.
func (c *getCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.getAPI()
	if err != nil {
		return err
	}
	defer apiclient.Close()

	results, err := apiclient.Get(c.ServiceName)
	if err != nil {
		return err
	}

	resultsMap := map[string]interface{}{
		"service":  results.Service,
		"charm":    results.Charm,
		"settings": results.Config,
	}
	return c.out.Write(ctx, resultsMap)
}
