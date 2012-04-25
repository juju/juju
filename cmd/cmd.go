package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/log"
	"os"
	"path/filepath"
	"strings"
)

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

// Context represents the run context of a Command. Command implementations
// should interpret file names relative to Dir (see AbsPath below), and print
// output and errors to Stdout and Stderr respectively.
type Context struct {
	Dir    string
	Stdout io.Writer
	Stderr io.Writer
}

// AbsPath returns an absolute representation of path, with relative paths
// interpreted as relative to ctx.Dir.
func (ctx *Context) AbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ctx.Dir, path)
}

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

// PrintHelp writes i's usage information and description to output, along with
// documentation for any flags defined in f. It calls f.SetOutput(ioutil.Discard).
func (i *Info) PrintHelp(output io.Writer, f *gnuflag.FlagSet) {
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

// Main runs the given Command in the supplied Context with the given
// arguments, which should not include the command name. It returns a code
// suitable for passing to os.Exit.
func Main(c Command, ctx *Context, args []string) int {
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.Usage = func() {}
	f.SetOutput(ioutil.Discard)
	printHelp := func() { c.Info().PrintHelp(ctx.Stderr, f) }
	printErr := func(err error) { fmt.Fprintf(ctx.Stderr, "ERROR: %v\n", err) }

	switch err := c.Init(f, args); err {
	case nil:
		if err = c.Run(ctx); err != nil {
			log.Debugf("%s command failed: %s\n", c.Info().Name, err)
			printErr(err)
			return 1
		}
	case gnuflag.ErrHelp:
		printHelp()
	default:
		printErr(err)
		printHelp()
		return 2
	}
	return 0
}

// DefaultContext returns a Context suitable for use in non-hosted situations.
func DefaultContext() *Context {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		panic(err)
	}
	return &Context{abs, os.Stdout, os.Stderr}
}

// CheckEmpty is a utility function that returns an error if args is not empty.
func CheckEmpty(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unrecognised args: %s", args)
	}
	return nil
}
