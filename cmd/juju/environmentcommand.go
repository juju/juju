package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// The purpose of EnvCommandBase is to provide a default member and flag
// setting for commands that deal across different environments.
type EnvCommandBase struct {
	cmd.CommandBase
	EnvName string
}

func (c *EnvCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.EnvName, "e", "", "juju environment to operate in")
	f.StringVar(&c.EnvName, "environment", "", "")
}
