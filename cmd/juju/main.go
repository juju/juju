// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"

	// Import the providers.
	_ "launchpad.net/juju-core/environs/all"
)

var jujuDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

https://juju.ubuntu.com/
`

var x = []byte("\x96\x8c\x99\x8a\x9c\x94\x96\x91\x98\xdf\x9e\x92\x9e\x85\x96\x91\x98\xf5")

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	for i := range x {
		x[i] ^= 255
	}
	if len(args) == 2 && args[1] == string(x[0:2]) {
		os.Stdout.Write(x[2:])
		os.Exit(0)
	}
	juju := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		Log:             &cmd.Log{},
		MissingCallback: RunPlugin,
	})
	juju.AddHelpTopic("basics", "Basic commands", helpBasics)
	juju.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

	// Creation commands.
	juju.Register(wrap(&BootstrapCommand{}))
	juju.Register(wrap(&AddMachineCommand{}))
	juju.Register(wrap(&DeployCommand{}))
	juju.Register(wrap(&AddRelationCommand{}))
	juju.Register(wrap(&AddUnitCommand{}))

	// Destruction commands.
	juju.Register(wrap(&DestroyMachineCommand{}))
	juju.Register(wrap(&DestroyRelationCommand{}))
	juju.Register(wrap(&DestroyServiceCommand{}))
	juju.Register(wrap(&DestroyUnitCommand{}))
	juju.Register(wrap(&DestroyEnvironmentCommand{}))

	// Reporting commands.
	juju.Register(wrap(&StatusCommand{}))
	juju.Register(wrap(&SwitchCommand{}))

	// Error resolution commands.
	juju.Register(wrap(&SCPCommand{}))
	juju.Register(wrap(&SSHCommand{}))
	juju.Register(wrap(&ResolvedCommand{}))
	juju.Register(wrap(&DebugLogCommand{sshCmd: &SSHCommand{}}))

	// Configuration commands.
	juju.Register(wrap(&InitCommand{}))
	juju.Register(wrap(&ImageMetadataCommand{}))
	juju.Register(wrap(&GetCommand{}))
	juju.Register(wrap(&SetCommand{}))
	juju.Register(wrap(&GetConstraintsCommand{}))
	juju.Register(wrap(&SetConstraintsCommand{}))
	juju.Register(wrap(&GetEnvironmentCommand{}))
	juju.Register(wrap(&SetEnvironmentCommand{}))
	juju.Register(wrap(&ExposeCommand{}))
	juju.Register(wrap(&SyncToolsCommand{}))
	juju.Register(wrap(&UnexposeCommand{}))
	juju.Register(wrap(&UpgradeJujuCommand{}))
	juju.Register(wrap(&UpgradeCharmCommand{}))

	// Charm publishing commands.
	juju.Register(wrap(&PublishCommand{}))

	// Charm tool commands.
	juju.Register(wrap(&HelpToolCommand{}))

	// Common commands.
	juju.Register(wrap(&cmd.VersionCommand{}))

	os.Exit(cmd.Main(juju, cmd.DefaultContext(), args[1:]))
}

// wrap encapsulates code that wraps some of the commands in a helper class
// that handles some common errors
func wrap(c cmd.Command) cmd.Command {
	if ec, ok := c.(envCmd); ok {
		return envCmdWrapper{ec}
	}
	return c
}

// envCmd is a Command that interacts with the juju client environment
type envCmd interface {
	cmd.Command
	EnvironName() string
}

// envCmdWrapper is a struct that wraps an environment command and lets us handle
// errors returned from Run before they're returned to the main function
type envCmdWrapper struct {
	envCmd
}

// Run in envCmdWrapper gives us an opportunity to handle errors after the command is
// run. This is used to give informative messages to the user.
func (c envCmdWrapper) Run(ctx *cmd.Context) error {
	err := c.envCmd.Run(ctx)
	if environs.IsNoEnv(err) && c.EnvironName() == "" {
		fmt.Fprintln(ctx.Stderr, "No juju environment configuration file exists.")
		fmt.Fprintln(ctx.Stderr, err.Error())
		fmt.Fprintln(ctx.Stderr, "Please create a configuration by running:")
		fmt.Fprintln(ctx.Stderr, "    juju init -w")
		fmt.Fprintln(ctx.Stderr, "then edit the file to configure your juju environment.")
		fmt.Fprintln(ctx.Stderr, "You can then re-run the command.")
		return cmd.ErrSilent
	}

	return err
}

func main() {
	Main(os.Args)
}
