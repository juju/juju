package main

import (
	"errors"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/statecmd"
)

// GetCommand retrieves the configuration of a service.
type GetCommand struct {
	EnvName     string
	ServiceName string
	out         cmd.Output
}

func (c *GetCommand) Info() *cmd.Info {
	return &cmd.Info{"get", "", "get service config options", ""}
}

func (c *GetCommand) SetFlags(f *gnuflag.FlagSet) {
	addEnvironFlags(&c.EnvName, f)
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
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	params := statecmd.ServiceGetParams{
		ServiceName: c.ServiceName,
	}

	results, err := statecmd.ServiceGet(conn.State, params)
	if err != nil {
		return err
	}

	resultsMap := map[string]interface{}{
		"service":  results.Service,
		"charm":    results.Charm,
		"settings": results.Settings,
	}
	return c.out.Write(ctx, resultsMap)
}
