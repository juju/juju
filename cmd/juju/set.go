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
	Options		[]Option
	ConfPath	string
}

type Option struct {
	Key, Value string
}

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{"set", "", "set service config options", ""}
}

func (c *SetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.StringVar(&c.ConfPath, "config", "", "path to yaml-formatted service config")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 || len(strings.Split(args[1], "=")) > 1 {
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
		v := strings.SplitN(opt, "=", 2)
		if len(v) != 2 { return nil, errors.New("invalid option") }
		o = append(o, Option{v[0], v[1]})
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
