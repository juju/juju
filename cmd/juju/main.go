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

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	juju := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:            "juju",
		Doc:             jujuDoc,
		Log:             &cmd.Log{},
		MissingCallback: RunPlugin,
	})
	juju.AddHelpTopic("basics", "Basic commands", helpBasics)

	// Creation commands.
	juju.Register(&BootstrapCommand{})
	juju.Register(&DeployCommand{})
	juju.Register(&AddRelationCommand{})
	juju.Register(&AddUnitCommand{})

	// Destruction commands.
	juju.Register(&DestroyMachineCommand{})
	juju.Register(&DestroyRelationCommand{})
	juju.Register(&DestroyServiceCommand{})
	juju.Register(&DestroyUnitCommand{})
	juju.Register(&DestroyEnvironmentCommand{})

	// Reporting commands.
	juju.Register(&StatusCommand{})
	juju.Register(&SwitchCommand{})

	// Error resolution commands.
	juju.Register(&SCPCommand{})
	juju.Register(&SSHCommand{})
	juju.Register(&ResolvedCommand{})
	juju.Register(&DebugLogCommand{sshCmd: &SSHCommand{}})

	// Configuration commands.
	juju.Register(&InitCommand{})
	juju.Register(&GetCommand{})
	juju.Register(&SetCommand{})
	juju.Register(&GetConstraintsCommand{})
	juju.Register(&SetConstraintsCommand{})
	juju.Register(&GetEnvironmentCommand{})
	juju.Register(&SetEnvironmentCommand{})
	juju.Register(&ExposeCommand{})
	juju.Register(&SyncToolsCommand{})
	juju.Register(&UnexposeCommand{})
	juju.Register(&UpgradeJujuCommand{})
	juju.Register(&UpgradeCharmCommand{})

	// Charm publishing commands.
	juju.Register(&PublishCommand{})

	// Common commands.
	juju.Register(&cmd.VersionCommand{})

	os.Exit(cmd.Main(juju, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
