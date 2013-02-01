package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"os"
)

// GenerateConfigCommand is used to write out a boilerplate environments.yaml file.
type GenerateConfigCommand struct {
}

func (c *GenerateConfigCommand) Info() *cmd.Info {
	return &cmd.Info{"generate-config", "", "generate boilerplate configuration for juju environments", ""}
}

func (c *GenerateConfigCommand) Init(f *gnuflag.FlagSet, args []string) error {
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run checks to see if there is already an environments.yaml file. In one exists already, it is
// a boilerplate version is created so that the user can edit it to get started.
func (c *GenerateConfigCommand) Run(context *cmd.Context) error {
	_, err := environs.ReadEnvirons("")
	out := context.Stdout
	if err != nil {
		if os.IsNotExist(err) {
			filename, err := environs.WriteEnvirons("", environs.BoilerPlateConfig())
			if err == nil {
				fmt.Fprintf(out, "A boilerplate environment configuration file has been written to %s.\n", filename)
				fmt.Fprint(out, "Edit the file to configure your juju environment and re-run bootstrap.\n")
				return nil
			} else {
				return fmt.Errorf("A boilerplate environment configuration file could not be created: %s", err.Error())
			}
		}
		return err
	} else {
		fmt.Fprintf(out, "A juju environment configuration already exists.\n")
		fmt.Fprintf(out, "It will not be overwritten.\n")
	}
	return nil
}
