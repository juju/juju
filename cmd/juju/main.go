// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"

	// Import the providers.
	_ "launchpad.net/juju-core/provider/all"
)

var logger = loggo.GetLogger("juju.cmd.juju")

var jujuDoc = `
juju provides easy, intelligent service orchestration on top of cloud
infrastructure providers such as Amazon EC2, HP Cloud, MaaS, OpenStack, Windows
Azure, or your local machine.

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
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		Log:             &cmd.Log{},
		MissingCallback: RunPlugin,
	})
	jujucmd.AddHelpTopic("basics", "Basic commands", helpBasics)
	jujucmd.AddHelpTopic("local", "How to configure a local (LXC) provider",
		helpProviderStart+helpLocalProvider+helpProviderEnd)
	jujucmd.AddHelpTopic("openstack", "How to configure an OpenStack provider",
		helpProviderStart+helpOpenstackProvider+helpProviderEnd)
	jujucmd.AddHelpTopic("ec2", "How to configure an Amazon EC2 provider",
		helpProviderStart+helpEC2Provider+helpProviderEnd)
	jujucmd.AddHelpTopic("hpcloud", "How to configure an HP Cloud provider",
		helpProviderStart+helpHPCloud+helpProviderEnd)
	jujucmd.AddHelpTopic("azure", "How to configure a Windows Azure provider",
		helpProviderStart+helpAzureProvider+helpProviderEnd)
	jujucmd.AddHelpTopic("constraints", "How to use commands with constraints", helpConstraints)
	jujucmd.AddHelpTopic("glossary", "Glossary of terms", helpGlossary)
	jujucmd.AddHelpTopic("logging", "How Juju handles logging", helpLogging)

	jujucmd.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

	// Creation commands.
	jujucmd.Register(wrap(&BootstrapCommand{}))
	jujucmd.Register(wrap(&AddMachineCommand{}))
	jujucmd.Register(wrap(&DeployCommand{}))
	jujucmd.Register(wrap(&AddRelationCommand{}))
	jujucmd.Register(wrap(&AddUnitCommand{}))

	// Destruction commands.
	jujucmd.Register(wrap(&DestroyMachineCommand{}))
	jujucmd.Register(wrap(&DestroyRelationCommand{}))
	jujucmd.Register(wrap(&DestroyServiceCommand{}))
	jujucmd.Register(wrap(&DestroyUnitCommand{}))
	jujucmd.Register(wrap(&DestroyEnvironmentCommand{}))

	// Reporting commands.
	jujucmd.Register(wrap(&StatusCommand{}))
	jujucmd.Register(wrap(&SwitchCommand{}))
	jujucmd.Register(wrap(&EndpointCommand{}))

	// Error resolution and debugging commands.
	jujucmd.Register(wrap(&RunCommand{}))
	jujucmd.Register(wrap(&SCPCommand{}))
	jujucmd.Register(wrap(&SSHCommand{}))
	jujucmd.Register(wrap(&ResolvedCommand{}))
	jujucmd.Register(wrap(&DebugLogCommand{sshCmd: &SSHCommand{}}))
	jujucmd.Register(wrap(&DebugHooksCommand{}))

	// Configuration commands.
	jujucmd.Register(wrap(&InitCommand{}))
	jujucmd.Register(wrap(&GetCommand{}))
	jujucmd.Register(wrap(&SetCommand{}))
	jujucmd.Register(wrap(&UnsetCommand{}))
	jujucmd.Register(wrap(&GetConstraintsCommand{}))
	jujucmd.Register(wrap(&SetConstraintsCommand{}))
	jujucmd.Register(wrap(&GetEnvironmentCommand{}))
	jujucmd.Register(wrap(&SetEnvironmentCommand{}))
	jujucmd.Register(wrap(&ExposeCommand{}))
	jujucmd.Register(wrap(&SyncToolsCommand{}))
	jujucmd.Register(wrap(&UnexposeCommand{}))
	jujucmd.Register(wrap(&UpgradeJujuCommand{}))
	jujucmd.Register(wrap(&UpgradeCharmCommand{}))

	// Charm publishing commands.
	jujucmd.Register(wrap(&PublishCommand{}))

	// Charm tool commands.
	jujucmd.Register(wrap(&HelpToolCommand{}))

	// Manage authorised ssh keys.
	jujucmd.Register(wrap(NewAuthorisedKeysCommand()))

	// Common commands.
	jujucmd.Register(wrap(&cmd.VersionCommand{}))

	os.Exit(cmd.Main(jujucmd, cmd.DefaultContext(), args[1:]))
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
		fmt.Fprintln(ctx.Stderr, err)
		fmt.Fprintln(ctx.Stderr, "Please create a configuration by running:")
		fmt.Fprintln(ctx.Stderr, "    juju init")
		fmt.Fprintln(ctx.Stderr, "then edit the file to configure your juju environment.")
		fmt.Fprintln(ctx.Stderr, "You can then re-run the command.")
		return cmd.ErrSilent
	}

	return err
}

func main() {
	Main(os.Args)
}
