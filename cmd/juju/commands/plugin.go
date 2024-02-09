// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/juju/osenv"
)

const JujuPluginPrefix = "juju-"
const JujuPluginPattern = "^juju-[a-zA-Z]"

var jujuArgNames = set.NewStrings("-m", "--model", "-c", "--controller")

// This is a very rudimentary method used to extract common Juju
// arguments from the full list passed to the plugin.
func extractJujuArgs(args []string) []string {
	var jujuArgs []string
	nrArgs := len(args)
	for nextArg := 0; nextArg < nrArgs; {
		arg := args[nextArg]
		nextArg++
		if !jujuArgNames.Contains(arg) {
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

// RunPlugin attempts to find the plugin on path to run
func RunPlugin(callback cmd.MissingCallback) cmd.MissingCallback {
	return func(ctx *cmd.Context, subcommand string, args []string) error {
		cmdName := JujuPluginPrefix + subcommand
		plugin := &PluginCommand{name: cmdName}

		// We process common flags supported by Juju commands.
		// To do this, we extract only those supported flags from the
		// argument list to avoid confusing flags.Parse().
		flags := gnuflag.NewFlagSetWithFlagKnownAs(cmdName, gnuflag.ContinueOnError, "option")
		flags.SetOutput(io.Discard)
		plugin.SetFlags(flags)
		jujuArgs := extractJujuArgs(args)
		if err := flags.Parse(false, jujuArgs); err != nil {
			return err
		}
		if err := plugin.Init(args); err != nil {
			return err
		}
		err := plugin.Run(ctx)
		_, execError := err.(*exec.Error)
		_, pathError := err.(*os.PathError)
		// exec.Error and pathError results are for when the executable isn't found, in
		// those cases, let's test whether we have a similar command available.
		if !execError && !pathError {
			return err
		}

		return callback(ctx, subcommand, args)
	}
}

type PluginCommand struct {
	cmd.CommandBase
	name string

	controllerName string
	modelName      string

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
	f.StringVar(&c.modelName, "m", "", "Model to operate in. Accepts [<controller name>:]<model name>")
	f.StringVar(&c.modelName, "model", "", "")
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
	c.CommandBase.SetFlags(f)
}

func (c *PluginCommand) Run(ctx *cmd.Context) error {
	command := exec.Command(c.name, c.args...)

	env := os.Environ()
	if c.controllerName != "" {
		env = utils.Setenv(env, osenv.JujuControllerEnvKey+"="+c.controllerName)
	}
	if c.modelName != "" {
		env = utils.Setenv(env, osenv.JujuModelEnvKey+"="+c.modelName)
	}
	command.Env = env

	// Now hook up stdin, stdout, stderr
	command.Stdin = ctx.Stdin
	command.Stdout = ctx.Stdout
	command.Stderr = ctx.Stderr
	// And run it!
	err := command.Run()

	if exitError, ok := err.(*exec.ExitError); ok && exitError != nil {
		status := exitError.ProcessState.Sys().(syscall.WaitStatus)
		if status.Exited() {
			return cmd.NewRcPassthroughError(status.ExitStatus())
		}
	}
	return err
}

type PluginDescription struct {
	name        string
	description string
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
				logger.Errorf("'%s --description': %s", plugin, err)
			}
		}(plugin)
	}
	resultMap := map[string]PluginDescription{}
	// gather the results at the end
	for range plugins {
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

// findPlugins searches the current PATH for executable files that match
// JujuPluginPattern.
func findPlugins() []string {
	re := regexp.MustCompile(JujuPluginPattern)
	path := os.Getenv("PATH")
	plugins := []string{}
	for _, name := range filepath.SplitList(path) {
		entries, err := os.ReadDir(name)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			fi, err := entry.Info()
			if err != nil {
				continue
			}

			if re.Match([]byte(fi.Name())) && (fi.Mode()&0111) != 0 {
				plugins = append(plugins, entry.Name())
			}
		}
	}
	sort.Strings(plugins)
	return plugins
}
