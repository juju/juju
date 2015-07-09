// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// GetCommand retrieves the configuration of a service.
type GetCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
	out         cmd.Output
}

const getDoc = `
The command output includes the service and charm names, a detailed list of all config
settings for <service>, including the setting name, whether it uses the default value
or not ("default: true"), description (if set), type, and current value. Example:

$ juju get wordpress

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

// Run fetches the configuration of the service and formats
// the result as a YAML string.
func (c *GetCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
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
