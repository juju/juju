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
	"syscall"
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
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

// DescriptionTimeout is how long we wait before killing the processes started
// to find the descriptions for the plugins.
var DescriptionTimeout = 1.0 * time.Second

type PluginDescription struct {
	name        string
	description string
	err         error
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

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			if err := cmd.Start(); err != nil {
				result.err = err
				return
			}

			done := make(chan struct{})

			go func() {
				select {
				case <-done:
					// everything is ok
				case <-time.After(DescriptionTimeout):
					result.description = "error: plugin took too long to respond"
					// Not killing fast enough.
					result.err = cmd.Process.Kill()
					fmt.Printf("%s killed: err: %#v\n", plugin, result.err)
					//result.err = cmd.Process.Signal(syscall.SIGINT)
					_ = syscall.SIGINT
				}
			}()
			defer close(done)
			result.err = cmd.Wait()
			if result.err == nil {
				// TODO: trim to only get the first line
				result.description = strings.SplitN(output.String(), "\n", 2)[0]
			} else {
				fmt.Println(result.err)
			}
		}(plugin, cmd)
	}
	resultMap := map[string]PluginDescription{}
	// gather the results at the end.
	for _ = range plugins {
		result := <-description
		fmt.Printf("%#v\n", result)
		resultMap[result.name] = result
	}
	fmt.Printf("%#v\n", resultMap)
	fmt.Printf("%#v\n", plugins)
	// plugins array is already sorted, use this to get the results in order
	for _, plugin := range plugins {
		results = append(results, resultMap[plugin])
	}

	fmt.Printf("%#v\n", results)
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
