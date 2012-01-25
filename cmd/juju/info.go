package main

import (
	"fmt"
	"os"
	"strings"
)

// Info holds everything necessary to describe a command's intent and usage.
type Info struct {
	name         string
	usage        string
	desc         string
	details      string
	printOptions func()
}

// Name will return the command name as used on the command line.
func (i *Info) Name() string {
	return i.name
}

// Desc will return a single-line description of the command.
func (i *Info) Desc() string {
	return i.desc
}

// PrintUsage will dump full usage information to os.Stderr.
func (i *Info) PrintUsage() {
	fmt.Fprintln(os.Stderr, "usage:", i.usage)
	if i.details != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", strings.TrimSpace(i.details))
	}
	fmt.Fprintln(os.Stderr, "\noptions:")
	i.printOptions()
}
