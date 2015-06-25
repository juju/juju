// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v1"
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
		"set":  formatGetForSet,
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

// formatGetForSet outputs the provided get structure in a format that can be used with
// `juju set`.
func formatGetForSet(cfgRaw interface{}) ([]byte, error) {
	if cfgRaw == nil {
		return nil, nil
	}
	cfg, ok := cfgRaw.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("unexpected value type: %T", cfgRaw)
	}

	serviceName, ok := cfg["service"].(string)
	if !ok {
		return nil, errors.Errorf("could not determine service name")
	}
	settings, ok := cfg["settings"].(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("could not determine service settings")
	}
	if settings == nil {
		return nil, nil
	}

	simpleSettings, err := simplifySettings(settings)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result, err := goyaml.Marshal(map[string]interface{}{
		serviceName: simpleSettings,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	result = []byte(strings.TrimRight(string(result), "\n"))
	return result, nil
}

func simplifySettings(cfg map[string]interface{}) (map[string]interface{}, error) {
	out := map[string]interface{}{}

	for key, settingsRaw := range cfg {
		settings, ok := settingsRaw.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("could not determine config for key %s", key)
		}
		out[key] = settings["value"]
	}
	return out, nil
}
