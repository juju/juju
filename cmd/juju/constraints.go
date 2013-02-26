package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
)

// GetConstraintsCommand shows the constraints for a service or environment.
type GetConstraintsCommand struct {
	cmd.CommandBase
	EnvName     string
	ServiceName string
	out         cmd.Output
}

func (c *GetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-constraints",
		Args:    "[<service>]",
		Purpose: "view constraints",
	}
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(state.Constraints).String()), nil
}

func (c *GetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	addEnvironFlags(&c.EnvName, f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
}

func (c *GetConstraintsCommand) Init(args []string) error {
	if len(args) > 0 {
		if !state.IsServiceName(args[0]) {
			return fmt.Errorf("invalid service name %q", args[0])
		}
		c.ServiceName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

func (c *GetConstraintsCommand) Run(ctx *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	var cons state.Constraints
	if c.ServiceName == "" {
		cons, err = conn.State.EnvironConstraints()
	} else {
		var svc *state.Service
		if svc, err = conn.State.Service(c.ServiceName); err != nil {
			return err
		}
		cons, err = svc.Constraints()
	}
	if err != nil {
		return err
	}
	return c.out.Write(ctx, cons)
}

// SetConstraintsCommand shows the constraints for a service or environment.
type SetConstraintsCommand struct {
	cmd.CommandBase
	EnvName     string
	ServiceName string
	Constraints state.Constraints
}

func (c *SetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-constraints",
		Args:    "[key=[value] ...]",
		Purpose: "replace constraints",
	}
}

func (c *SetConstraintsCommand) SetFlags(f *gnuflag.FlagSet) {
	addEnvironFlags(&c.EnvName, f)
	f.StringVar(&c.ServiceName, "s", "", "set service constraints")
	f.StringVar(&c.ServiceName, "service", "", "")
}

func (c *SetConstraintsCommand) Init(args []string) (err error) {
	if c.ServiceName != "" && !state.IsServiceName(c.ServiceName) {
		return fmt.Errorf("invalid service name %q", c.ServiceName)
	}
	c.Constraints, err = state.ParseConstraints(args...)
	return err
}

func (c *SetConstraintsCommand) Run(_ *cmd.Context) (err error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	if c.ServiceName == "" {
		return conn.State.SetEnvironConstraints(c.Constraints)
	}
	var svc *state.Service
	if svc, err = conn.State.Service(c.ServiceName); err != nil {
		return err
	}
	return svc.SetConstraints(c.Constraints)
}
