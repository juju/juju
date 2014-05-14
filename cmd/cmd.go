// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"
)

type RcPassthroughError struct {
	Code int
}

func (e *RcPassthroughError) Error() string {
	return fmt.Sprintf("subprocess encountered error code %v", e.Code)
}

func IsRcPassthroughError(err error) bool {
	_, ok := err.(*RcPassthroughError)
	return ok
}

// NewRcPassthroughError creates an error that will have the code used at the
// return code from the cmd.Main function rather than the default of 1 if
// there is an error.
func NewRcPassthroughError(code int) error {
	return &RcPassthroughError{code}
}

// ErrSilent can be returned from Run to signal that Main should exit with
// code 1 without producing error output.
var ErrSilent = errors.New("cmd: error out silently")

// Command is implemented by types that interpret command-line arguments.
type Command interface {
	// IsSuperCommand returns true if the command is a super command.
	IsSuperCommand() bool

	// Info returns information about the Command.
	Info() *Info

	// SetFlags adds command specific flags to the flag set.
	SetFlags(f *gnuflag.FlagSet)

	// Init initializes the Command before running.
	Init(args []string) error

	// Run will execute the Command as directed by the options and positional
	// arguments passed to Init.
	Run(ctx *Context) error

	// AllowInterspersedFlags returns whether the command allows flag
	// arguments to be interspersed with non-flag arguments.
	AllowInterspersedFlags() bool
}

// CommandBase provides the default implementation for SetFlags, Init, and Help.
type CommandBase struct{}

// IsSuperCommand implements Command.IsSuperCommand
func (c *CommandBase) IsSuperCommand() bool {
	return false
}

// SetFlags does nothing in the simplest case.
func (c *CommandBase) SetFlags(f *gnuflag.FlagSet) {}

// Init in the simplest case makes sure there are no args.
func (c *CommandBase) Init(args []string) error {
	return CheckEmpty(args)
}

// AllowInterspersedFlags returns true by default. Some subcommands
// may want to override this.
func (c *CommandBase) AllowInterspersedFlags() bool {
	return true
}

// Context represents the run context of a Command. Command implementations
// should interpret file names relative to Dir (see AbsPath below), and print
// output and errors to Stdout and Stderr respectively.
type Context struct {
	Dir     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	quiet   bool
	verbose bool
}

func (ctx *Context) write(format string, params ...interface{}) {
	output := fmt.Sprintf(format, params...)
	if !strings.HasSuffix(output, "\n") {
		output = output + "\n"
	}
	fmt.Fprint(ctx.Stderr, output)
}

// Infof will write the formatted string to Stderr if quiet is false, but if
// quiet is true the message is logged.
func (ctx *Context) Infof(format string, params ...interface{}) {
	if ctx.quiet {
		logger.Infof(format, params...)
	} else {
		ctx.write(format, params...)
	}
}

// Verbosef will write the formatted string to Stderr if the verbose is true,
// and to the logger if not.
func (ctx *Context) Verbosef(format string, params ...interface{}) {
	if ctx.verbose {
		ctx.write(format, params...)
	} else {
		logger.Infof(format, params...)
	}
}

// AbsPath returns an absolute representation of path, with relative paths
// interpreted as relative to ctx.Dir.
func (ctx *Context) AbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ctx.Dir, path)
}

// GetStdin satisfies environs.BootstrapContext
func (ctx *Context) GetStdin() io.Reader {
	return ctx.Stdin
}

// GetStdout satisfies environs.BootstrapContext
func (ctx *Context) GetStdout() io.Writer {
	return ctx.Stdout
}

// GetStderr satisfies environs.BootstrapContext
func (ctx *Context) GetStderr() io.Writer {
	return ctx.Stderr
}

// InterruptNotify satisfies environs.BootstrapContext
func (ctx *Context) InterruptNotify(c chan<- os.Signal) {
	signal.Notify(c, os.Interrupt)
}

// StopInterruptNotify satisfies environs.BootstrapContext
func (ctx *Context) StopInterruptNotify(c chan<- os.Signal) {
	signal.Stop(c)
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

	// Aliases are other names for the Command.
	Aliases []string
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
	if len(i.Aliases) > 0 {
		fmt.Fprintf(buf, "\naliases: %s\n", strings.Join(i.Aliases, ", "))
	}
	return buf.Bytes()
}

// Errors from commands can be ErrSilent (don't print an error message),
// ErrHelp (show the help) or some other error related to needed flags
// missing, or needed positional args missing, in which case we should
// print the error and return a non-zero return code.
func handleCommandError(c Command, ctx *Context, err error, f *gnuflag.FlagSet) (rc int, done bool) {
	switch err {
	case nil:
		return 0, false
	case gnuflag.ErrHelp:
		ctx.Stdout.Write(c.Info().Help(f))
		return 0, true
	case ErrSilent:
		return 2, true
	default:
		fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		return 2, true
	}
}

// Main runs the given Command in the supplied Context with the given
// arguments, which should not include the command name. It returns a code
// suitable for passing to os.Exit.
func Main(c Command, ctx *Context, args []string) int {
	f := gnuflag.NewFlagSet(c.Info().Name, gnuflag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	c.SetFlags(f)
	if rc, done := handleCommandError(c, ctx, f.Parse(c.AllowInterspersedFlags(), args), f); done {
		return rc
	}
	// Since SuperCommands can also return gnuflag.ErrHelp errors, we need to
	// handle both those types of errors as well as "real" errors.
	if rc, done := handleCommandError(c, ctx, c.Init(f.Args()), f); done {
		return rc
	}
	if err := c.Run(ctx); err != nil {
		if IsRcPassthroughError(err) {
			return err.(*RcPassthroughError).Code
		}
		if err != ErrSilent {
			fmt.Fprintf(ctx.Stderr, "error: %v\n", err)
		}
		return 1
	}
	return 0
}

// DefaultContext returns a Context suitable for use in non-hosted situations.
func DefaultContext() (*Context, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &Context{
		Dir:    abs,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, nil
}

// CheckEmpty is a utility function that returns an error if args is not empty.
func CheckEmpty(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unrecognized args: %q", args)
	}
	return nil
}

// ZeroOrOneArgs checks to see that there are zero or one args, and returns
// the value of the arg if provided, or the empty string if not.
func ZeroOrOneArgs(args []string) (string, error) {
	var result string
	if len(args) > 0 {
		result, args = args[0], args[1:]
	}
	if err := CheckEmpty(args); err != nil {
		return "", err
	}
	return result, nil
}
