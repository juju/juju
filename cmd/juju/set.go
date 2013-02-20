package main

import (
	"errors"
	"fmt"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/statecmd"
)

// SetCommand updates the configuration of a service
type SetCommand struct {
	EnvName     string
	ServiceName string
	// either Options or Config will contain the configuration data
	Options []string
	Config  cmd.FileVar
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
	if len(c.Config.Path) > 0 && len(args) > 1 {
		return errors.New("cannot specify --config when using key=value arguments")
	}
	c.ServiceName, c.Options = args[0], args[1:]
	return nil
}

// Run updates the configuration of a service
func (c *SetCommand) Run(ctx *cmd.Context) error {
	contents, err := c.Config.Read(ctx)
	if err != nil && err != cmd.ErrNoPath {
		return err
	}
	var options map[string]string
	if len(contents) == 0 {
		if len(c.Options) == 0 {
			// nothing to do.
			return nil
		}
		options, err = parse(c.Options)
		if err != nil {
			return err
		}
	}
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	return statecmd.ServiceSet(conn.State, statecmd.ServiceSetParams{
		ServiceName: c.ServiceName,
		Options:     options,
		Config:      string(contents),
	})
}

// parse parses the option k=v strings into a map of options to be
// updated in the config. Keys with empty values are returned separately
// and should be removed.
func parse(options []string) (map[string]string, error) {
	kv := make(map[string]string)
	for _, o := range options {
		s := strings.SplitN(o, "=", 2)
		if len(s) != 2 || s[0] == "" {
			return nil, fmt.Errorf("invalid option: %q", o)
		}
		kv[s[0]] = s[1]
	}
	return kv, nil
}
