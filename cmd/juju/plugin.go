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
)

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	plugin := &PluginCommand{name: "juju-" + subcommand}

	f := gnuflag.NewFlagSet(subcommand, gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	plugin.SetFlags(f)
	cmd.ParseArgs(plugin, f, args)
	plugin.Init(f.Args())
	return plugin.Run(ctx)
}

type PluginCommand struct {
	EnvCommandBase
	name string
	args []string
}

// PluginCommand implements this solely to implement cmd.Command
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
			return fmt.Errorf("couldn't read the environment")
		}
		env = environments.Default
	}

	os.Setenv("JUJU_ENV", env)
	command := exec.Command(c.name, c.args...)

	// Now hook up stdin, stdout, stderr
	command.Stdin = ctx.Stdin
	command.Stdout = ctx.Stdout
	command.Stderr = ctx.Stderr

	return command.Run()
}

func findPlugins() []string {
	path := os.Getenv("PATH")
	plugins := []string{}
	for _, name := range filepath.SplitList(path) {
		fullpath := filepath.Join(name, "juju-*")
		matches, err := filepath.Glob(fullpath)
		// If this errors we don't care and continue
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			// Again, if stat fails, we don't care
			if err != nil {
				continue
			}
			// Don't be too anal about the exec bit, but check to see if it is executable.
			if (info.Mode() & 0111) != 0 {
				plugins = append(plugins, filepath.Base(match))
			}
		}
	}
	sort.Strings(plugins)
	return plugins
}
