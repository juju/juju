// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"os"
)

// Import the providers.
import (
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
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		Log:             &cmd.Log{},
		MissingCallback: RunPlugin,
	})
	jujucmd.AddHelpTopic("basics", "Basic commands", helpBasics)
	jujucmd.AddHelpTopic("local", "How to configure a local (LXC) provider", helpLocalProvider)
	jujucmd.AddHelpTopic("openstack", "How to configure an OpenStack provider", helpOpenstackProvider)
	jujucmd.AddHelpTopic("aws", "How to configure an AWS (EC2) provider", helpEC2Provider)
	jujucmd.AddHelpTopic("hpcloud", "How to configure an HP Cloud provider", helpHPCloud)
	jujucmd.AddHelpTopic("glossary", "Glossary of terms", helpGlossary)

	jujucmd.AddHelpTopicCallback("plugins", "Show Juju plugins", PluginHelpTopic)

	// Creation commands.
	jujucmd.Register(&BootstrapCommand{})
	jujucmd.Register(&AddMachineCommand{})
	jujucmd.Register(&DeployCommand{})
	jujucmd.Register(&AddRelationCommand{})
	jujucmd.Register(&AddUnitCommand{})

	// Destruction commands.
	jujucmd.Register(&DestroyMachineCommand{})
	jujucmd.Register(&DestroyRelationCommand{})
	jujucmd.Register(&DestroyServiceCommand{})
	jujucmd.Register(&DestroyUnitCommand{})
	jujucmd.Register(&DestroyEnvironmentCommand{})

	// Reporting commands.
	jujucmd.Register(&StatusCommand{})
	jujucmd.Register(&SwitchCommand{})

	// Error resolution and debugging commands.
	jujucmd.Register(&SCPCommand{})
	jujucmd.Register(&SSHCommand{})
	jujucmd.Register(&ResolvedCommand{})
	jujucmd.Register(&DebugLogCommand{sshCmd: &SSHCommand{}})
	jujucmd.Register(&DebugHooksCommand{})

	// Configuration commands.
	jujucmd.Register(&InitCommand{})
	jujucmd.Register(&GetCommand{})
	jujucmd.Register(&SetCommand{})
	jujucmd.Register(&GetConstraintsCommand{})
	jujucmd.Register(&SetConstraintsCommand{})
	jujucmd.Register(&GetEnvironmentCommand{})
	jujucmd.Register(&SetEnvironmentCommand{})
	jujucmd.Register(&ExposeCommand{})
	jujucmd.Register(&SyncToolsCommand{})
	jujucmd.Register(&UnexposeCommand{})
	jujucmd.Register(&UpgradeJujuCommand{})
	jujucmd.Register(&UpgradeCharmCommand{})

	// Charm publishing commands.
	jujucmd.Register(&PublishCommand{})

	// Charm tool commands.
	jujucmd.Register(&HelpToolCommand{})

	// Common commands.
	jujucmd.Register(&cmd.VersionCommand{})

	os.Exit(cmd.Main(jujucmd, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
