// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/loggo"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju"
	// Import the providers.
	_ "github.com/juju/juju/provider/all"
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
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err = juju.InitJujuHome(); err != nil {
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
	jcmd := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		MissingCallback: RunPlugin,
	})
	jcmd.AddHelpTopic("basics", "Basic commands", helpBasics)
	jcmd.AddHelpTopic("local-provider", "How to configure a local (LXC) provider",
		helpProviderStart+helpLocalProvider+helpProviderEnd)
	jcmd.AddHelpTopic("openstack-provider", "How to configure an OpenStack provider",
		helpProviderStart+helpOpenstackProvider+helpProviderEnd, "openstack")
	jcmd.AddHelpTopic("ec2-provider", "How to configure an Amazon EC2 provider",
		helpProviderStart+helpEC2Provider+helpProviderEnd, "ec2", "aws", "amazon")
	jcmd.AddHelpTopic("hpcloud-provider", "How to configure an HP Cloud provider",
		helpProviderStart+helpHPCloud+helpProviderEnd, "hpcloud", "hp-cloud")
	jcmd.AddHelpTopic("azure-provider", "How to configure a Windows Azure provider",
		helpProviderStart+helpAzureProvider+helpProviderEnd, "azure")
	jcmd.AddHelpTopic("constraints", "How to use commands with constraints", helpConstraints)
	jcmd.AddHelpTopic("glossary", "Glossary of terms", helpGlossary)
	jcmd.AddHelpTopic("logging", "How Juju handles logging", helpLogging)

	jcmd.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

	registerCommands(jcmd, ctx)
	os.Exit(cmd.Main(jcmd, ctx, args[1:]))
}

type commandRegistry interface {
	Register(cmd.Command)
}

// registerCommands registers commands in the specified registry.
// EnvironCommands must be wrapped with an envCmdWrapper.
func registerCommands(r commandRegistry, ctx *cmd.Context) {
	wrapEnvCommand := func(c envcmd.EnvironCommand) cmd.Command {
		return envCmdWrapper{envcmd.Wrap(c), ctx}
	}

	// Creation commands.
	r.Register(wrapEnvCommand(&BootstrapCommand{}))
	r.Register(wrapEnvCommand(&AddMachineCommand{}))
	r.Register(wrapEnvCommand(&DeployCommand{}))
	r.Register(wrapEnvCommand(&AddRelationCommand{}))
	r.Register(wrapEnvCommand(&AddUnitCommand{}))

	// Destruction commands.
	r.Register(wrapEnvCommand(&RemoveMachineCommand{}))
	r.Register(wrapEnvCommand(&RemoveRelationCommand{}))
	r.Register(wrapEnvCommand(&RemoveServiceCommand{}))
	r.Register(wrapEnvCommand(&RemoveUnitCommand{}))
	r.Register(&DestroyEnvironmentCommand{})

	// Reporting commands.
	r.Register(wrapEnvCommand(&StatusCommand{}))
	r.Register(&SwitchCommand{})
	r.Register(wrapEnvCommand(&EndpointCommand{}))

	// Error resolution and debugging commands.
	r.Register(wrapEnvCommand(&RunCommand{}))
	r.Register(wrapEnvCommand(&SCPCommand{}))
	r.Register(wrapEnvCommand(&SSHCommand{}))
	r.Register(wrapEnvCommand(&ResolvedCommand{}))
	r.Register(wrapEnvCommand(&DebugLogCommand{}))
	r.Register(wrapEnvCommand(&DebugHooksCommand{}))
	r.Register(wrapEnvCommand(&RetryProvisioningCommand{}))

	// Configuration commands.
	r.Register(&InitCommand{})
	r.Register(wrapEnvCommand(&GetCommand{}))
	r.Register(wrapEnvCommand(&SetCommand{}))
	r.Register(wrapEnvCommand(&UnsetCommand{}))
	r.Register(wrapEnvCommand(&GetConstraintsCommand{}))
	r.Register(wrapEnvCommand(&SetConstraintsCommand{}))
	r.Register(wrapEnvCommand(&GetEnvironmentCommand{}))
	r.Register(wrapEnvCommand(&SetEnvironmentCommand{}))
	r.Register(wrapEnvCommand(&UnsetEnvironmentCommand{}))
	r.Register(wrapEnvCommand(&ExposeCommand{}))
	r.Register(wrapEnvCommand(&SyncToolsCommand{}))
	r.Register(wrapEnvCommand(&UnexposeCommand{}))
	r.Register(wrapEnvCommand(&UpgradeJujuCommand{}))
	r.Register(wrapEnvCommand(&UpgradeCharmCommand{}))

	// Charm publishing commands.
	r.Register(wrapEnvCommand(&PublishCommand{}))

	// Charm tool commands.
	r.Register(&HelpToolCommand{})

	// Manage backups.
	r.Register(backups.NewCommand())

	// Manage authorized ssh keys.
	r.Register(NewAuthorizedKeysCommand())

	// Manage users and access
	r.Register(NewUserCommand())

	// Manage state server availability.
	r.Register(wrapEnvCommand(&EnsureAvailabilityCommand{}))
}

// envCmdWrapper is a struct that wraps an environment command and lets us handle
// errors returned from Init before they're returned to the main function.
type envCmdWrapper struct {
	cmd.Command
	ctx *cmd.Context
}

func (w envCmdWrapper) Init(args []string) error {
	err := w.Command.Init(args)
	if environs.IsNoEnv(err) {
		fmt.Fprintln(w.ctx.Stderr, "No juju environment configuration file exists.")
		fmt.Fprintln(w.ctx.Stderr, err)
		fmt.Fprintln(w.ctx.Stderr, "Please create a configuration by running:")
		fmt.Fprintln(w.ctx.Stderr, "    juju init")
		fmt.Fprintln(w.ctx.Stderr, "then edit the file to configure your juju environment.")
		fmt.Fprintln(w.ctx.Stderr, "You can then re-run the command.")
		return cmd.ErrSilent
	}
	return err
}

func main() {
	Main(os.Args)
}
