// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"errors"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

// GetCommand retrieves the configuration of a service.
type GetCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	out         cmd.Output
	api         GetServiceAPI
}

const getDoc = `
The command output includes the service and charm names, a detailed list of all config
settings for <service>, including the setting name, whether it uses the default value
or not ("default: true"), description (if set), type, and current value. Example:

$ juju service get wordpress

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

NOTE: In the example above the descriptions and most other settings were omitted for
brevity. The "engine" setting was left at its default value ("nginx"), while the
"tuning" setting was set to "optimized" (the default value is "single").
`

func (c *GetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get",
		Args:    "<service>",
		Purpose: "get service configuration options",
		Doc:     getDoc,
	}
}

func (c *GetCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO(dfc) add json formatting ?
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

func (c *GetCommand) Init(args []string) error {
	// TODO(dfc) add --schema-only
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// GetServiceAPI defines the methods on the client API
// that the service get command calls.
type GetServiceAPI interface {
	Close() error
	ServiceGet(service string) (*params.ServiceGetResults, error)
}

func (c *GetCommand) getAPI() (GetServiceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Run fetches the configuration of the service and formats
// the result as a YAML string.
func (c *GetCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.ServiceGet(c.ServiceName)
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
