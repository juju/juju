// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

type SwitchCommand struct {
	cmd.CommandBase
	EnvName string
	List    bool
}

var switchDoc = `
Show or change the default juju environment name.

If no command line parameters are passed, switch will output the current
environment as defined by the file $JUJU_HOME/current-environment.

If a command line parameter is passed in, that value will is stored in the
current environment file if it represents a valid environment name as
specified in the environments.yaml file.
`

func (c *SwitchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch",
		Args:    "[environment name]",
		Purpose: "show or change the default juju environment name",
		Doc:     switchDoc,
		Aliases: []string{"env"},
	}
}

func (c *SwitchCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.List, "l", false, "list the environment names")
	f.BoolVar(&c.List, "list", false, "")
}

func (c *SwitchCommand) Init(args []string) (err error) {
	c.EnvName, err = cmd.ZeroOrOneArgs(args)
	return
}

func getConfigstoreEnvironments() (set.Strings, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get config store")
	}
	other, err := store.List()
	if err != nil {
		return nil, errors.Annotate(err, "failed to list environments in config store")
	}
	return set.NewStrings(other...), nil
}

func (c *SwitchCommand) Run(ctx *cmd.Context) error {
	// Switch is an alternative way of dealing with environments than using
	// the JUJU_ENV environment setting, and as such, doesn't play too well.
	// If JUJU_ENV is set we should report that as the current environment,
	// and not allow switching when it is set.

	// Passing through the empty string reads the default environments.yaml file.
	environments, err := environs.ReadEnvirons("")
	if err != nil {
		return errors.Errorf("couldn't read the environment")
	}

	names := set.NewStrings(environments.Names()...)
	configEnvirons, err := getConfigstoreEnvironments()
	if err != nil {
		return err
	}
	names = names.Union(configEnvirons)

	if c.List {
		// List all environments.
		if c.EnvName != "" {
			return errors.New("cannot switch and list at the same time")
		}
		for _, name := range names.SortedValues() {
			fmt.Fprintf(ctx.Stdout, "%s\n", name)
		}
		return nil
	}

	jujuEnv := os.Getenv("JUJU_ENV")
	if jujuEnv != "" {
		if c.EnvName == "" {
			fmt.Fprintf(ctx.Stdout, "%s\n", jujuEnv)
			return nil
		} else {
			return errors.Errorf("cannot switch when JUJU_ENV is overriding the environment (set to %q)", jujuEnv)
		}
	}

	currentEnv := envcmd.ReadCurrentEnvironment()
	if currentEnv == "" {
		currentEnv = environments.Default
	}

	// Handle the different operation modes.
	switch {
	case c.EnvName == "" && currentEnv == "":
		// Nothing specified and nothing to switch to.
		return errors.New("no currently specified environment")
	case c.EnvName == "":
		// Simply print the current environment.
		fmt.Fprintf(ctx.Stdout, "%s\n", currentEnv)
	default:
		// Switch the environment.
		if !names.Contains(c.EnvName) {
			return errors.Errorf("%q is not a name of an existing defined environment", c.EnvName)
		}
		if err := envcmd.WriteCurrentEnvironment(c.EnvName); err != nil {
			return err
		}
		if currentEnv == "" {
			fmt.Fprintf(ctx.Stdout, "-> %s\n", c.EnvName)
		} else {
			fmt.Fprintf(ctx.Stdout, "%s -> %s\n", currentEnv, c.EnvName)
		}
	}
	return nil
}
