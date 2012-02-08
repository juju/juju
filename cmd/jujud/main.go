package main

import (
	"fmt"
	"launchpad.net/juju/go/cmd"
	"os"
)

var (
	jujudDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://juju.ubuntu.com/`
)

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	jc := cmd.NewSuperCommand("jujud", jujudDoc)
	jc.Register(&InitzkCommand{})
	jc.Register(NewAgentCommand(NewUnitFlags()))
	jc.Register(NewAgentCommand(NewMachineFlags()))
	jc.Register(NewAgentCommand(NewProvisioningFlags()))
	cmd.Main(jc, args)
}

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

func main() {
	Main(os.Args)
}
