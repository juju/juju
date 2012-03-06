package cmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"os"
	"strings"
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

	// Intersperse controls whether the Command will accept interspersed
	// options and positional args.
	Intersperse bool
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

	// InitFlagSet prepares a FlagSet such that Parse~ing that FlagSet will
	// initialize the Command's options.
	InitFlagSet(f *gnuflag.FlagSet)

	// ParsePositional is called by Parse to allow the Command to handle
	// positional command-line arguments.
	ParsePositional(args []string) error

	// Run will execute the Command according to the options and positional
	// arguments interpreted by a call to Parse.
	Run(ctx *Context) error
}

// newFlagSet returns a FlagSet initialized for use with c.
func newFlagSet(c Command, output io.Writer) *gnuflag.FlagSet {
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.SetOutput(output)
	f.Usage = func() { printUsage(c, output) }
	c.InitFlagSet(f)
	return f
}

// options directs PrintDefaults on c's FlagSet to a temporary buffer and
// returns its contents. Since FlagSet doesn't conveniently expose .formal,
// this appears to be the best way to find out whether any options are actually
// available.
func options(c Command) string {
	optOut := bytes.NewBuffer([]byte{})
	f := newFlagSet(c, optOut)
	f.PrintDefaults()
	return strings.TrimSpace(optOut.String())
}

// printUsage prints usage information for c to output.
func printUsage(c Command, output io.Writer) {
	i := c.Info()
	fmt.Fprintf(output, "usage: %s\n", i.Usage())
	if i.Purpose != "" {
		fmt.Fprintf(output, "purpose: %s\n", i.Purpose)
	}
	if opt := options(c); opt != "" {
		fmt.Fprintf(output, "\noptions:\n%s\n", opt)
	}
	if i.Doc != "" {
		fmt.Fprintf(output, "\n%s\n", strings.TrimSpace(i.Doc))
	}
}

// Parse parses args on c. This must be called before c is Run.
func Parse(c Command, args []string) error {
	// If we use nil instead of Discard, gnuflag will interpret that as meaning
	// "print to os.Stderr"; but our intent is to entirely suppress gnuflag's
	// interactions with the os package, and handle to handle and all errors in
	// exactly the same way, regardless of source.
	f := newFlagSet(c, ioutil.Discard)
	if err := f.Parse(c.Info().Intersperse, args); err != nil {
		return err
	}
	return c.ParsePositional(f.Args())
}

// CheckEmpty is a utility function that returns an error if args is not empty.
func CheckEmpty(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unrecognised args: %s", args)
	}
	return nil
}

// Main will Parse and Run a Command in a default context suitable for a
// command-line tool, and will call os.Exit with the appropriate code.
func Main(c Command, args []string) {
	os.Exit(DefaultContext().Main(c, args[1:]))
}
