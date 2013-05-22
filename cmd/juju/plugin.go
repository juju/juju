// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
)

const JujuPluginPrefix = "juju-"

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	plugin := &PluginCommand{name: JujuPluginPrefix + subcommand}

	flags := gnuflag.NewFlagSet(subcommand, gnuflag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	plugin.SetFlags(flags)
	cmd.ParseArgs(plugin, flags, args)
	plugin.Init(flags.Args())
	err := plugin.Run(ctx)
	_, execError := err.(*exec.Error)
	// exec.Error results are for when the executable isn't found, in
	// those cases, drop through.
	if !execError {
		return err
	}
	return fmt.Errorf("unrecognized command: juju %s", subcommand)
}

type PluginCommand struct {
	EnvCommandBase
	name string
	args []string
}

// Info is just a stub so that PluginCommand implements cmd.Command.
// Since this is never actually called, we can happily return nil.
func (*PluginCommand) Info() *cmd.Info {
	return nil
}

func (c *PluginCommand) Init(args []string) error {
	c.args = args
	return nil
}

func (c *PluginCommand) Run(ctx *cmd.Context) error {
	env := c.EnvName
	if env == "" {
		// Passing through the empty string reads the default environments.yaml file.
		environments, err := environs.ReadEnvirons("")
		if err != nil {
			log.Errorf("could not read the environments.yaml file: %s", err)
			return fmt.Errorf("could not read the environments.yaml file")
		}
		env = environments.Default
	}

	os.Setenv("JUJU_ENV", env)
	command := exec.Command(c.name, c.args...)

	// Now hook up stdin, stdout, stderr
	command.Stdin = ctx.Stdin
	command.Stdout = ctx.Stdout
	command.Stderr = ctx.Stderr
	// And run it!
	return command.Run()
}

// findPlugins searches the current PATH for executable files that start with
// JujuPluginPrefix.
func findPlugins() []string {
	path := os.Getenv("PATH")
	plugins := []string{}
	for _, name := range filepath.SplitList(path) {
		entries, err := ioutil.ReadDir(name)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), JujuPluginPrefix) && (entry.Mode()&0111) != 0 {
				plugins = append(plugins, entry.Name())
			}
		}
	}
	sort.Strings(plugins)
	return plugins
}
