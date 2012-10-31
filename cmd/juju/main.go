package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"os"
)

// When we import an environment provider implementation
// here, it will register itself with environs, and hence
// be available to the juju command.
import (
	_ "launchpad.net/juju-core/environs/ec2"
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
	juju := &cmd.SuperCommand{Name: "juju", Doc: jujuDoc, Log: &cmd.Log{}}
	juju.Register(&AddRelationCommand{})
	juju.Register(&AddUnitCommand{})
	juju.Register(&BootstrapCommand{})
	juju.Register(&DeployCommand{})
	juju.Register(&DestroyEnvironmentCommand{})
	juju.Register(&ExposeCommand{})
	juju.Register(&GetCommand{})
	juju.Register(&RemoveRelationCommand{})
	juju.Register(&RemoveUnitCommand{})
	juju.Register(&ResolvedCommand{})
	juju.Register(&SetCommand{})
	juju.Register(&SCPCommand{})
	juju.Register(&SSHCommand{})
	juju.Register(&StatusCommand{})
	juju.Register(&UnexposeCommand{})
	juju.Register(&UpgradeJujuCommand{})
	os.Exit(cmd.Main(juju, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}

func addEnvironFlags(name *string, f *gnuflag.FlagSet) {
	f.StringVar(name, "e", "", "juju environment to operate in")
	f.StringVar(name, "environment", "", "")
}
