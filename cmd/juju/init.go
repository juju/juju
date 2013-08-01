// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
)

// InitCommand is used to write out a boilerplate environments.yaml file.
type InitCommand struct {
	cmd.CommandBase
	WriteFile bool
}

func (c *InitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "init",
		Purpose: "generate boilerplate configuration for juju environments",
		Aliases: []string{"generate-config"},
	}
}

func (c *InitCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.WriteFile, "w", false, "write to environments.yaml file if it doesn't already exist")
}

// Run checks to see if there is already an environments.yaml file. In one does not exist already,
// a boilerplate version is created so that the user can edit it to get started.
func (c *InitCommand) Run(context *cmd.Context) error {
	out := context.Stdout
	config := environs.BoilerplateConfig()
	if !c.WriteFile {
		fmt.Fprintln(out, config)
		return nil
	}
	_, err := environs.ReadEnvirons("")
	if err == nil {
		fmt.Fprintf(out, "A juju environment configuration already exists.\n")
		fmt.Fprintf(out, "It will not be overwritten.\n")
		return nil
	}
	if !errors.IsNoEnv(err) {
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
