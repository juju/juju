package main

import (
	"fmt"
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
	if err := jc.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
