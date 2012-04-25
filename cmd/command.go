package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"strings"
)

// Info holds everything necessary to describe a Command's intent and usage,
// excluding information about the specific flags accepted; this information is
// stored in a separate FlagSet, which may include additional flags injected by
// support code, and cannot be determined by examining a Command in isolation.
type Info struct {
	// Name is the Command's name.
	Name string

	// Args describes the command's expected positional arguments.
	Args string

	// Purpose is a short explanation of the Command's purpose.
	Purpose string

	// Doc is the long documentation for the Command.
	Doc string
}

// printUsage writes i's content to output, along with documentation for any
// flags defined in f. It calls f.SetOutput(ioutil.Discard).
func (i *Info) printUsage(output io.Writer, f *gnuflag.FlagSet) {
	fmt.Fprintf(output, "usage: %s", i.Name)
	hasOptions := false
	f.VisitAll(func(f *gnuflag.Flag) { hasOptions = true })
	if hasOptions {
		fmt.Fprintf(output, " [options]")
	}
	if i.Args != "" {
		fmt.Fprintf(output, " %s", i.Args)
	}
	if i.Purpose != "" {
		fmt.Fprintf(output, "\npurpose: %s", i.Purpose)
	}
	if hasOptions {
		fmt.Fprintf(output, "\n\noptions:\n")
		f.SetOutput(output)
		f.PrintDefaults()
	}
	f.SetOutput(ioutil.Discard)
	if i.Doc != "" {
		fmt.Fprintf(output, "\n%s", strings.TrimSpace(i.Doc))
	}
	fmt.Fprintf(output, "\n")
}

// Command is implemented by types that interpret command-line arguments.
type Command interface {
	// Info returns information about the Command.
	Info() *Info

	// Init initializes the Command before running. The command may add options
	// to f before processing args.
	Init(f *gnuflag.FlagSet, args []string) error

	// Run will execute the Command as directed by the options and positional
	// arguments passed to Init.
	Run(ctx *Context) error
}

// CheckEmpty is a utility function that returns an error if args is not empty.
func CheckEmpty(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unrecognised args: %s", args)
	}
	return nil
}
