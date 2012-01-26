package main

import (
	"fmt"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
	"os"
	"strings"
)

// Info holds everything necessary to describe a Command's intent and usage.
type Info struct {
	// Name is the Command's name.
	Name string

	// Usage describes the format of a valid call to the Command.
	Usage string

	// Purpose is a short explanation of the Command's purpose.
	Purpose string

	// Doc is the long documentation for the Command.
	Doc string
}

// Command is implemented by types that interpret any command-line arguments
// passed to the "juju" command.
type Command interface {
	// Info returns information about the command.
	Info() *Info

	// InitFlagSet prepares a FlagSet such that Parse~ing that FlagSet will
	// initialize the Command's options.
	InitFlagSet(f *flag.FlagSet)

	// ParsePositional is called by Parse to allow the Command to handle
	// positional command-line arguments.
	ParsePositional(args []string) error

	// Run will execute the command according to the options and positional
	// arguments interpreted by a call to Parse.
	Run() error
}

// NewFlagSet returns a FlagSet initialized for use with c.
func NewFlagSet(c Command) *flag.FlagSet {
	f := flag.NewFlagSet(c.Info().Name, flag.ExitOnError)
	f.Usage = func() { PrintUsage(c) }
	c.InitFlagSet(f)
	return f
}

// PrintUsage prints usage information for c to stderr.
func PrintUsage(c Command) {
	i := c.Info()
	fmt.Fprintf(os.Stderr, "usage: %s\n", i.Usage)
	fmt.Fprintf(os.Stderr, "purpose: %s\n", i.Purpose)
	fmt.Fprintf(os.Stderr, "\noptions:\n")
	NewFlagSet(c).PrintDefaults()
	if i.Doc != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", strings.TrimSpace(i.Doc))
	}
}

// Parse prepares c for Run~ning.
func Parse(c Command, intersperse bool, args []string) error {
	f := NewFlagSet(c)
	if err := f.Parse(intersperse, args); err != nil {
		return err
	}
	return c.ParsePositional(f.Args())
}
