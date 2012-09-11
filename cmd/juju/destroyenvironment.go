package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	EnvName string
}

func (c *DestroyEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		"destroy-environment", "[options]",
		"terminate all machines and other associated resources for an environment",
		"",
	}
}

func (c *DestroyEnvironmentCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *DestroyEnvironmentCommand) Run(_ *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}
	return environ.Destroy(nil)
}
