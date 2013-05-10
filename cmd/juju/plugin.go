package main

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/cmd"
)

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	return fmt.Errorf("unrecognized command: juju %s", subcommand)
}

type PluginCommand struct {
	EnvCommandBase
}

// PluginCommand implements this solely to implement cmd.Command
func (*PluginCommand) Info() *cmd.Info {
	return nil
}

func (c *PluginCommand) Run(ctx *cmd.Context) error {
	return nil
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
	return plugins
}
