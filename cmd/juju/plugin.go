// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/log"
)

const JujuPluginPrefix = "juju-"

// This is a very rudimentary method used to extract common Juju
// arguments from the full list passed to the plugin. Currently,
// there is only one such argument: -e env
// If more than just -e is required, the method can be improved then.
func extractJujuArgs(args []string) []string {
	var jujuArgs []string
	nrArgs := len(args)
	for nextArg := 0; nextArg < nrArgs; {
		arg := args[nextArg]
		nextArg++
		if arg != "-e" {
			continue
		}
		jujuArgs = append(jujuArgs, arg)
		if nextArg < nrArgs {
			jujuArgs = append(jujuArgs, args[nextArg])
			nextArg++
		}
	}
	return jujuArgs
}

func RunPlugin(ctx *cmd.Context, subcommand string, args []string) error {
	cmdName := JujuPluginPrefix + subcommand
	plugin := &PluginCommand{name: cmdName}

	// We process common flags supported by Juju commands.
	// To do this, we extract only those supported flags from the
	// argument list to avoid confusing flags.Parse().
	flags := gnuflag.NewFlagSet(cmdName, gnuflag.ContinueOnError)
	flags.SetOutput(ioutil.Discard)
	plugin.SetFlags(flags)
	jujuArgs := extractJujuArgs(args)
	err := flags.Parse(false, jujuArgs)
	if err != nil {
		return err
	}

	plugin.Init(args)
	err = plugin.Run(ctx)
	_, execError := err.(*exec.Error)
	// exec.Error results are for when the executable isn't found, in
	// those cases, drop through.
	if !execError {
		return err
	}
	return &cmd.UnrecognizedCommand{subcommand}
}

type PluginCommand struct {
	cmd.EnvCommandBase
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

func (c *PluginCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
}

func (c *PluginCommand) Run(ctx *cmd.Context) error {
	command := exec.Command(c.name, c.args...)
	command.Env = append(os.Environ(), []string{
		osenv.JujuHomeEnvKey + "=" + osenv.JujuHome(),
		osenv.JujuEnvEnvKey + "=" + c.EnvironName()}...,
	)

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

const PluginTopicText = `Juju Plugins

Plugins are implemented as stand-alone executable files somewhere in the user's PATH.
The executable command must be of the format juju-<plugin name>.

`

func PluginHelpTopic() string {
	output := &bytes.Buffer{}
	fmt.Fprintf(output, PluginTopicText)

	existingPlugins := GetPluginDescriptions()

	if len(existingPlugins) == 0 {
		fmt.Fprintf(output, "No plugins found.\n")
	} else {
		longest := 0
		for _, plugin := range existingPlugins {
			if len(plugin.name) > longest {
				longest = len(plugin.name)
			}
		}
		for _, plugin := range existingPlugins {
			fmt.Fprintf(output, "%-*s  %s\n", longest, plugin.name, plugin.description)
		}
	}

	return output.String()
}

// GetPluginDescriptions runs each plugin with "--description".  The calls to
// the plugins are run in parallel, so the function should only take as long
// as the longest call.
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
		go func(plugin string) {
			result := PluginDescription{name: plugin}
			defer func() {
				description <- result
			}()
			desccmd := exec.Command(plugin, "--description")
			output, err := desccmd.CombinedOutput()

			if err == nil {
				// trim to only get the first line
				result.description = strings.SplitN(string(output), "\n", 2)[0]
			} else {
				result.description = fmt.Sprintf("error occurred running '%s --description'", plugin)
				log.Errorf("'%s --description': %s", plugin, err)
			}
		}(plugin)
	}
	resultMap := map[string]PluginDescription{}
	// gather the results at the end
	for _ = range plugins {
		result := <-description
		resultMap[result.name] = result
	}
	// plugins array is already sorted, use this to get the results in order
	for _, plugin := range plugins {
		// Strip the 'juju-' off the start of the plugin name in the results
		result := resultMap[plugin]
		result.name = result.name[len(JujuPluginPrefix):]
		results = append(results, result)
	}
	return results
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
