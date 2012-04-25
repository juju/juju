package cmd

import (
	"fmt"
	"launchpad.net/gnuflag"
)

// Info holds everything necessary to describe a Command's intent and usage.
type Info struct {
	// Name is the Command's name.
	Name string

	// Args describes the command's expected arguments.
	Args string

	// Purpose is a short explanation of the Command's purpose.
	Purpose string

	// Doc is the long documentation for the Command.
	Doc string
}

// Usage combines Name and Args to describe the Command's intended usage.
func (i *Info) Usage() string {
	if i.Args == "" {
		return i.Name
	}
	return fmt.Sprintf("%s %s", i.Name, i.Args)
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
