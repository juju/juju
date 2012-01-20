package main

import (
    "fmt"
    "launchpad.net/juju/go/log"
    "os"
)

var subcommands = map[string]Command{
	"bootstrap": new(BootstrapCommand),
}

func main() {
	jc := new(JujuCommand)
	for name, subcmd := range subcommands {
		jc.Register(name, subcmd)
	}
	if err := jc.Parse(os.Args); err != nil {
		fmt.Println(jc.Usage())
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
