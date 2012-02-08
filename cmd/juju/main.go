package main

import (
	"launchpad.net/juju/go/cmd"
	"os"
)

var (
	jujuDoc = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

https://juju.ubuntu.com/`
)

// Main registers subcommands for the juju executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	jc := cmd.NewSuperCommand("juju", jujuDoc)
	jc.Register(&BootstrapCommand{})
	cmd.Main(jc, args)
}

func main() {
	Main(os.Args)
}
