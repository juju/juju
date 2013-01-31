package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"os"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	EnvName     string
	UploadTools bool
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{"bootstrap", "", "start up an environment from scratch", ""}
}

func (c *BootstrapCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// a boilerplate version is created so that the user can edit it to get started.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		if _, ok := err.(*os.PathError); ok {
			fmt.Println("No juju enviroment configuration file exists.")
			filename, err := environs.WriteEnvirons("", environs.BoilerPlateConfig())
			if err == nil {
				fmt.Printf("A boilerplate environment configuration file has been written to %s.\n", filename)
				fmt.Println("Edit the file to configure your juju environment and re-run bootstrap.")
				return nil
			} else {
				return fmt.Errorf("A boilerplate environment configurtion file could not be created: %s", err.Error())
			}

		}
		return err
	}
	return environs.Bootstrap(environ, c.UploadTools, nil)
}
