package server

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	*HookContext
	Key string // The key to show. If empty, show all.
	out cmd.Output
}

func NewConfigGetCommand(ctx *HookContext) (cmd.Command, error) {
	return &ConfigGetCommand{HookContext: ctx}, nil
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"config-get", "[<key>]",
		"print service configuration",
		"If a key is given, only the value for that key will be printed.",
	}
}

func (c *ConfigGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if args == nil {
		return nil
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *ConfigGetCommand) Run(ctx *cmd.Context) error {
	conf, err := c.Service.Config()
	if err != nil {
		return err
	}
	charm, _, err := c.Service.Charm()	
	if err != nil {
		return err
	}
	cfg, err := charm.Config().Validate(nil)
	if err != nil {
		return err
	}
	cfg = merge(conf.Map(), cfg)
	var value interface{}
	if c.Key == "" {
		value = cfg
	} else {
		value, _ = cfg[c.Key]
	}
	return c.out.Write(ctx, value)
}

func merge(a, b map[string]interface{}) map[string]interface{} {
	for k, v := range b {
		if _, ok := a[k]; !ok {
			a[k] = v
		}
	}
	return a
}
