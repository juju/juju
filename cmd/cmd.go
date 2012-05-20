package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/log"
	"os"
	"path/filepath"
	"strings"
)

// ErrSilent can be returned from Run to signal that Main should exit with
// code 1 without producing error output.
var ErrSilent = errors.New("cmd: error out silently")

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

// Help renders i's content, along with documentation for any
// flags defined in f. It calls f.SetOutput(ioutil.Discard).
func (i *Info) Help(f *gnuflag.FlagSet) []byte {
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
	fmt.Fprintf(buf, "\n")
	if i.Purpose != "" {
		fmt.Fprintf(buf, "purpose: %s\n", i.Purpose)
	}
	if hasOptions {
		fmt.Fprintf(buf, "\noptions:\n")
		f.SetOutput(buf)
		f.PrintDefaults()
	}
	f.SetOutput(ioutil.Discard)
	if i.Doc != "" {
		fmt.Fprintf(buf, "\n%s\n", strings.TrimSpace(i.Doc))
	}
	return buf.Bytes()
}

// Main runs the given Command in the supplied Context with the given
// arguments, which should not include the command name. It returns a code
// suitable for passing to os.Exit.
func Main(c Command, ctx *Context, args []string) int {
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	if err := c.Init(f, args); err != nil {
		ctx.Stderr.Write(c.Info().Help(f))
		if err == gnuflag.ErrHelp {
			return 0
		}
		fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		return 2
	}
	if err := c.Run(ctx); err != nil {
		if err != ErrSilent {
			log.Debugf("%s command failed: %s\n", c.Info().Name, err)
			fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		}
		return 1
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
		return fmt.Errorf("unrecognized args: %q", args)
	}
	return nil
}
