package main

import (
	"fmt"
	"launchpad.net/juju/go/log"
	"os"
)

var subcommands = map[string]Command{
	"bootstrap": &BootstrapCommand{},
}

func main() {
	Main(os.Args)
}

func Main(args []string) {
	jc := &JujuCommand{}
	for name, subcmd := range subcommands {
		jc.Register(name, subcmd)
	}
	if err := jc.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		jc.PrintUsage()
		os.Exit(2)
	}
	log.Debug = jc.Verbose()
	if err := log.SetFile(jc.Logfile()); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
	if err := jc.Run(); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
