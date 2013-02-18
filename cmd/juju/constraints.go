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
	EnvName     string
	ServiceName string
	out         cmd.Output
}

func (c *GetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{"get-constraints", "", "view constraints", ""}
}

func formatConstraints(value interface{}) ([]byte, error) {
	return []byte(value.(state.Constraints).String()), nil
}

func (c *GetConstraintsCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	c.out.AddFlags(f, "constraints", map[string]cmd.Formatter{
		"constraints": formatConstraints,
		"yaml":        cmd.FormatYaml,
		"json":        cmd.FormatJson,
	})
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
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
	EnvName     string
	ServiceName string
	Constraints state.Constraints
}

func (c *SetConstraintsCommand) Info() *cmd.Info {
	return &cmd.Info{"set-constraints", "[key=[value],...]", "replace constraints", ""}
}

func (c *SetConstraintsCommand) Init(f *gnuflag.FlagSet, args []string) (err error) {
	addEnvironFlags(&c.EnvName, f)
	f.StringVar(&c.ServiceName, "s", "", "set service constraints")
	f.StringVar(&c.ServiceName, "service", "", "")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.ServiceName != "" && !state.IsServiceName(c.ServiceName) {
		return fmt.Errorf("invalid service name %q", c.ServiceName)
	}
	c.Constraints, err = state.ParseConstraints(f.Args()...)
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
