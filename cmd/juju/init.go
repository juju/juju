// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// InitCommand is used to write out a boilerplate environments.yaml file.
type InitCommand struct {
	cmd.CommandBase
	WriteFile bool
	Show      bool
}

func (c *InitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "init",
		Purpose: "generate boilerplate configuration for juju environments",
		Aliases: []string{"generate-config"},
	}
}

func (c *InitCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.WriteFile, "f", false, "force overwriting environments.yaml file even if it exists (ignored if --show flag specified)")
	f.BoolVar(&c.Show, "show", false, "print the generated configuration data to stdout instead of writing it to a file")
}

var errJujuEnvExists = fmt.Errorf(`A juju environment configuration already exists.

Use -f to overwrite the existing environments.yaml.
`)

// Run checks to see if there is already an environments.yaml file. In one does not exist already,
// a boilerplate version is created so that the user can edit it to get started.
func (c *InitCommand) Run(context *cmd.Context) error {
	out := context.Stdout
	config := environs.BoilerplateConfig()
	if c.Show {
		fmt.Fprint(out, config)
		return nil
	}
	_, err := environs.ReadEnvirons("")
	if err == nil && !c.WriteFile {
		return errJujuEnvExists
	}
	if err != nil && !environs.IsNoEnv(err) {
		return err
	}
	filename, err := environs.WriteEnvirons("", config)
	if err != nil {
		return fmt.Errorf("A boilerplate environment configuration file could not be created: %s", err.Error())
	}
	fmt.Fprintf(out, "A boilerplate environment configuration file has been written to %s.\n", filename)
	fmt.Fprint(out, "Edit the file to configure your juju environment and run bootstrap.\n")
	return nil
}
