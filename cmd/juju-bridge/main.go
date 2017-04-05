// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/juju/juju/network/debinterfaces"
	"github.com/juju/utils/clock"
)

const usage = `
Bridge existing devices

usage: [ -p ] [ -b <bridge-prefix ] <filename> <device-name>~<bridge-name>...

Options:

  -p -- parse and print to stdout, no activation

Example:

  $ juju-bridge /etc/network/interfaces ens3~br-ens3 bond0.150~br-bond0.150
`

func printParseError(err error) {
	if pe, ok := err.(*debinterfaces.ParseError); ok {
		fmt.Printf("error: %q:%d: %s: %s\n", pe.Filename, pe.LineNum, pe.Line, pe.Message)
	} else {
		fmt.Printf("error: %v\n", err)
	}
}

func main() {
	parseOnlyFlag := flag.Bool("p", false, "parse and print to stdout, no activation")

	flag.Parse()
	args := flag.Args()

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	if *parseOnlyFlag {
		stanzas, err := debinterfaces.Parse(args[0])

		if err != nil {
			printParseError(err)
			os.Exit(1)
		}

		fmt.Println(debinterfaces.FormatStanzas(debinterfaces.FlattenStanzas(stanzas), 4))
		os.Exit(0)
	}

	devices := make(map[string]string)
	for _, v := range args[1:] {
		arg := strings.Split(v, "~")
		if len(arg) != 2 {
			fmt.Fprintln(os.Stderr, usage)
			os.Exit(1)
		}
		devices[arg[0]] = arg[1]
	}

	params := debinterfaces.ActivationParams{
		Clock:            clock.WallClock,
		Filename:         args[0],
		Devices:          devices,
		ReconfigureDelay: 10,
		Timeout:          5 * time.Minute,
	}

	result, err := debinterfaces.BridgeAndActivate(params)

	if err != nil {
		printParseError(err)
		os.Exit(1)
	} else if result != nil {
		if result.Code != 0 {
			if len(result.Stdout) > 0 {
				fmt.Fprintln(os.Stderr, string(result.Stdout))
			}
			if len(result.Stderr) > 0 {
				fmt.Fprintln(os.Stderr, string(result.Stderr))
			}
		}
		os.Exit(result.Code)
	}
}
