package main

import (
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"os"
	"path/filepath"
)

// When we import an environment provider implementation
// here, it will register itself with environs, and hence
// be available to the juju command.
import (
	_ "launchpad.net/juju-core/environs/ec2"
	_ "launchpad.net/juju-core/environs/openstack"
)

var jujuDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

https://juju.ubuntu.com/
`

// checkJujuHome retrieves $JUJU_HOME or $HOME to set the juju home.
// In case both variables aren't set the command will exit with an
// error. 
func checkJujuHome() {
	jujuHome := os.Getenv("JUJU_HOME")
	if jujuHome == "" {
		home := os.Getenv("HOME")
		if home == "" {
			fmt.Fprintf(os.Stderr, "command failed: cannot determine juju home, neither $JUJU_HOME nor $HOME are set")
			os.Exit(1)
		}
		jujuHome = filepath.Join(home, ".juju")
	}
	config.SetJujuHome(jujuHome)
}

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	checkJujuHome()
	juju := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "juju",
		Doc:  jujuDoc,
		Log:  &cmd.Log{},
	})
	juju.AddHelpTopic("basics", "Basic commands", helpBasics)

	// Register creation commands.
	juju.Register(&BootstrapCommand{})
	juju.Register(&DeployCommand{})
	juju.Register(&AddRelationCommand{})
	juju.Register(&AddUnitCommand{})

	// Register destruction commands.
	juju.Register(&DestroyMachineCommand{})
	juju.Register(&DestroyRelationCommand{})
	juju.Register(&DestroyServiceCommand{})
	juju.Register(&DestroyUnitCommand{})
	juju.Register(&DestroyEnvironmentCommand{})

	// Register error resolution commands.
	juju.Register(&StatusCommand{})
	juju.Register(&SCPCommand{})
	juju.Register(&SSHCommand{})
	juju.Register(&ResolvedCommand{})

	// Register configuration commands.
	juju.Register(&InitCommand{})
	juju.Register(&GetCommand{})
	juju.Register(&SetCommand{})
	juju.Register(&GetConstraintsCommand{})
	juju.Register(&SetConstraintsCommand{})
	juju.Register(&ExposeCommand{})
	juju.Register(&UnexposeCommand{})
	juju.Register(&UpgradeJujuCommand{})
	juju.Register(&UpgradeCharmCommand{})

	// register common commands
	juju.Register(&cmd.VersionCommand{})

	os.Exit(cmd.Main(juju, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
