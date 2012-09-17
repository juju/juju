package main

import (
	"errors"
	"reflect"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
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

func (c *GetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	// TODO(dfc) add json formatting ?
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
	// TODO(dfc) add --schema-only
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
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
	svc, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	svcfg, err := svc.Config()
	if err != nil {
		return err
	}
	charm, _, err := svc.Charm()
	if err != nil {
		return err
	}
	chcfg := charm.Config().Options

	config := merge(svcfg.Map(), chcfg)

	result := map[string]interface{}{
		"service":  svc.Name(),
		"charm":    charm.Meta().Name,
		"settings": config,
	}
	return c.out.Write(ctx, result)
}

// merge service settings and charm schema
func merge(svcfg map[string]interface{}, chcfg map[string]charm.Option) map[string]interface{} {
	r := make(map[string]interface{})
	for k, v := range chcfg {
		m := map[string]interface{}{
			"description": v.Description,
			"type":        v.Type,
		}
		if s, ok := svcfg[k]; ok {
			m["value"] = s
		} else {
			// breaks compatibility with py/juju
			m["value"] = nil
		}
		if v.Default != nil {
			if reflect.DeepEqual(v.Default, svcfg[k]) {
				m["default"] = true
			}
		}
		r[k] = m
	}
	return r
}
