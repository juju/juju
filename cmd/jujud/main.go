package main

import (
	"launchpad.net/juju/go/cmd"
	"os"
)

var jujudDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal. jujud is a component of juju.

https://juju.ubuntu.com/
`

// Main registers subcommands for the jujud executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	jujud := &cmd.SuperCommand{Name: "jujud", Doc: jujudDoc, Log: &cmd.Log{}}
	jujud.Register(&InitzkCommand{})
	jujud.Register(&UnitAgent{})
	jujud.Register(&MachineAgent{})
	jujud.Register(NewProvisioningAgent())
	os.Exit(cmd.Main(jujud, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
