package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
)

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	plugin := &PluginCommand{name: "juju-" + subcommand}

	flags := gnuflag.NewFlagSet(subcommand, gnuflag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	plugin.SetFlags(flags)
	cmd.ParseArgs(plugin, flags, args)
	plugin.Init(flags.Args())
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
	// And run it!
	return command.Run()
}

type PluginDescription struct {
	name        string
	description string
}

// GetPluginDescriptions runs each plugin with "--description".  If the plugin
// takes longer than DescriptionTimeout to return, the subprocess is killed
// and the description becomes an error message.
func GetPluginDescriptions() []PluginDescription {
	plugins := findPlugins()
	results := []PluginDescription{}
	if len(plugins) == 0 {
		return results
	}
	// create a channel with enough backing for each plugin
	description := make(chan PluginDescription, len(plugins))

	// exec the command, and wait only for the timeout before killing the process
	for _, plugin := range plugins {
		cmd := exec.Command(plugin, "--description")
		go func(plugin string, cmd *exec.Cmd) {
			result := PluginDescription{name: plugin}
			defer func() {
				description <- result
			}()
			output, err := cmd.CombinedOutput()

			if err == nil {
				// trim to only get the first line
				result.description = strings.SplitN(string(output), "\n", 2)[0]
			} else {
				result.description = fmt.Sprintf("error occurred running '%s --description'", plugin)
				log.Errorf("'%s --description': %s", plugin, err)
			}
		}(plugin, cmd)
	}
	resultMap := map[string]PluginDescription{}
	// gather the results at the end.
	for _ = range plugins {
		result := <-description
		resultMap[result.name] = result
	}
	// plugins array is already sorted, use this to get the results in order
	for _, plugin := range plugins {
		results = append(results, resultMap[plugin])
	}
	return results
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
