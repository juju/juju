package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"strings"
)

// Info holds some of the usage documentation of a Command.
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

// help renders i's content, along with documentation for any
// flags defined in f. It calls f.SetOutput(ioutil.Discard).
func (i *Info) help(f *gnuflag.FlagSet) []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "usage: %s", i.Name)
	hasOptions := false
	f.VisitAll(func(f *gnuflag.Flag) { hasOptions = true })
	if hasOptions {
		fmt.Fprintf(buf, " [options]")
	}
	if i.Args != "" {
		fmt.Fprintf(buf, " %s", i.Args)
	}
	if i.Purpose != "" {
		fmt.Fprintf(buf, "\npurpose: %s", i.Purpose)
	}
	if hasOptions {
		fmt.Fprintf(buf, "\n\noptions:\n")
		f.SetOutput(buf)
		f.PrintDefaults()
	}
	f.SetOutput(ioutil.Discard)
	if i.Doc != "" {
		fmt.Fprintf(buf, "\n%s", strings.TrimSpace(i.Doc))
	}
	fmt.Fprintf(buf, "\n")
	return buf.Bytes()
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
