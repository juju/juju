package main

import (
	"fmt"
	"io"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
	"os"
	"strings"
)

// Info holds everything necessary to describe a Command's intent and usage.
type Info struct {
	Name        string
	Usage       string
	Description string
	Details     string
	PrintMore   func(io.Writer)
}

type Command interface {
	// Info returns a description of the command.
	Info() *Info

	// InitFlagSet prepares a FlagSet such that Parse~ing that FlagSet will
	// initialize the Command's options.
	InitFlagSet(f *flag.FlagSet)

	// Unconsumed is called by Parse to allow the Command to handle positional
	// command-line arguments.
	Unconsumed(args []string) error

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
	fmt.Fprintln(os.Stderr, "usage:", i.Usage)
	if i.Details != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", strings.TrimSpace(i.Details))
	}
	fmt.Fprintln(os.Stderr, "\noptions:")
	NewFlagSet(c).PrintDefaults()
	if i.PrintMore != nil {
		i.PrintMore(os.Stderr)
	}
}

// Parse prepares c for Run~ning.
func Parse(c Command, intersperse bool, args []string) error {
	f := NewFlagSet(c)
	if err := f.Parse(intersperse, args); err != nil {
		return err
	}
	return c.Unconsumed(f.Args())
}
