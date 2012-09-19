package main

import (
	"errors"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// SetCommand updates the configuration of a service
type SetCommand struct {
	EnvName     string
	ServiceName string
	Options     []Option
	Config      cmd.FileVar
}

type Option struct {
	Key, Value string
}

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{"set", "", "set service config options", ""}
}

func (c *SetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	var err error
	c.Options, err = parseOptions(args[1:])
	return err
}

func parseOptions(opts []string) ([]Option, error) {
	var o []Option
	for _, opt := range opts {
		s := strings.SplitN(opt, "=", 2)
		if len(s) != 2 {
			return nil, errors.New("invalid option")
		}
		k, v := strings.TrimSpace(s[0]), strings.TrimSpace(s[1])
		if len(k) == 0 {
			return nil, errors.New("missing option key")
		}
		if len(v) == 0 {
			return nil, errors.New("missing option value")
		}
		o = append(o, Option{k, v})
	}
	return o, nil
}

// Run updates the configuration of a service
func (c *SetCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}
