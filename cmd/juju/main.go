package main

import (
	"fmt"
	"launchpad.net/juju/go/log"
	"os"
)

func main() {
	Main(os.Args)
}

func Main(args []string) {
	jc := NewJujuCommand()
	jc.Register(&BootstrapCommand{})

	if err := Parse(jc, false, args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		PrintUsage(jc)
		os.Exit(2)
	}
	log.Debug = jc.Verbose
	if err := log.SetFile(jc.Logfile); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
	if err := jc.Run(); err != nil {
		log.Printf("%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
