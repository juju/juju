// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/ansiterm"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4"

	corelogger "github.com/juju/juju/core/logger"
)

// ErrSilent can be returned from Run to signal that Main should exit with
// code 1 without producing error output.
var ErrSilent = errors.New("cmd: error out silently")

// IsErrSilent returns whether the error should be logged from cmd.Main.
func IsErrSilent(err error) bool {
	if err == ErrSilent {
		return true
	}
	if utils.IsRcPassthroughError(err) {
		return true
	}
	return false
}

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
	context.Context
	Dir              string
	Env              map[string]string
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	outputFormatUsed bool
	quiet            bool
	verbose          bool
	serialisable     bool
}

// With returns a command context with the specified context.Context.
func (ctx *Context) With(c context.Context) *Context {
	newCtx := *ctx
	newCtx.Context = c
	return &newCtx
}

// Quiet reports whether the command is in "quiet" mode. When
// this is true, informational output should be suppressed (logger
// messages can be used instead).
func (ctx *Context) Quiet() bool {
	return ctx.quiet
}

// IsSerial reports whether the command is required to output to a "machine".
// This mode is intended to stop the proliferation of execessive writes to
// stdout and stderr, when the output is intended for machines.
func (ctx *Context) IsSerial() bool {
	return ctx.serialisable
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
		//Here we use the Loggo.logger method `Logf` as opposed to
		//`logger.Infof` to avoid introducing an additional call stack
		//level (since `Infof` calls `Logf` internally). This is done so
		//that this function can produce more accurate source location
		//debug information.
		logger.Logf(ctx, corelogger.INFO, corelogger.Labels{}, format, params...)
	} else {
		ctx.write(format, params...)
	}
}

// Warningf allows for the logging of messages, at the warning level, from a
// command's context. This is useful for logging errors which do not cause a
// command to fail (e.g. an error message used as a deprecation warning that
// will be upgraded to a real error message at some point in the future.)
func (ctx *Context) Warningf(format string, params ...interface{}) {
	// Here we use the Loggo.logger method `Logf` as opposed to
	// `logger.Warningf` to avoid introducing an additional call stack level
	// (since `Warningf` calls Logf internally). This is done so that this
	// function can produce more accurate source location debug information.
	logger.Logf(ctx, corelogger.WARNING, corelogger.Labels{}, format, params...)
}

// Verbosef will write the formatted string to Stderr if the verbose is true,
// and to the logger if not.
func (ctx *Context) Verbosef(format string, params ...interface{}) {
	if ctx.verbose {
		ctx.write(format, params...)
	} else {
		// Here we use the Loggo.logger method `Logf` as opposed to
		// `logger.Infof` to avoid introducing an additional call stack
		// level (since `Infof` calls `Logf` internally). This is done so
		// that this function can produce more accurate source location
		// debug information.
		logger.Logf(ctx, corelogger.INFO, corelogger.Labels{}, format, params...)
	}
}

// Errorf allows for the logging of error messages from a command's
// context. This should be used for errors which cause a command to fail.
// Usually these errors are logged by returning them in Run, but that is
// not always sufficient. For instance, if the client has performed multiple
// actions
func (ctx *Context) Errorf(format string, params ...interface{}) {
	// Here we use the Loggo.logger method `Logf` as opposed to
	// `logger.Errorf` to avoid introducing an additional call stack
	// level (since `Errorf` calls `Logf` internally). This is done so
	// that this function can produce more accurate source location
	// debug information.
	logger.Logf(ctx, corelogger.ERROR, corelogger.Labels{}, format, params...)
}

// WriteError will output the formatted text to the writer with
// a colored ERROR like the logging would.
//
// Deprecated: Use ctx.Errorf instead
func WriteError(writer io.Writer, err error) {
	w := ansiterm.NewWriter(writer)
	ansiterm.Foreground(ansiterm.BrightRed).Fprintf(w, "ERROR")
	fmt.Fprintf(w, " %s\n", err.Error())
}

// Getenv looks up an environment variable in the context. It mirrors
// os.Getenv. An empty string is returned if the key is not set.
func (ctx *Context) Getenv(key string) string {
	value, _ := ctx.Env[key]
	return value
}

// Setenv sets an environment variable in the context. It mirrors os.Setenv.
func (ctx *Context) Setenv(key, value string) error {
	if ctx.Env == nil {
		ctx.Env = make(map[string]string)
	}
	ctx.Env[key] = value
	return nil
}

// AbsPath returns an absolute representation of path, with relative paths
// interpreted as relative to ctx.Dir and with "~/" replaced with users
// home dir.
func (ctx *Context) AbsPath(path string) string {
	if normalizedPath, err := utils.NormalizePath(path); err == nil {
		path = normalizedPath
	}
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

	// Subcommands stores the name and description of each subcommand.
	Subcommands map[string]string

	// Examples is a collection of running examples.
	Examples string

	// SeeAlso is a collection of additional commands to be checked.
	SeeAlso []string

	// Aliases are other names for the Command.
	Aliases []string

	// FlagKnownAs allows different projects to customise what their flags are
	// known as, e.g. 'flag', 'option', 'item'. All error/log messages
	// will use that name when referring to an individual items/flags in this command.
	// For example, if this value is 'option', the default message 'value for flag'
	// will become 'value for option'.
	FlagKnownAs string

	// ShowSuperFlags contains the names of the 'super' command flags
	// that are desired to be shown in the sub-command help output.
	ShowSuperFlags []string
}

// Help renders i's content, along with documentation for any
// flags defined in f.
func (i *Info) Help(f *gnuflag.FlagSet) []byte {
	return i.HelpWithSuperFlags(nil, f)
}

// HelpWithSuperFlags renders i's content, along with documentation for any
// flags defined in both command and its super command flag sets.
// Only super command flags defined in i.ShowSuperFlags are displayed, if found.
func (i *Info) HelpWithSuperFlags(superF *gnuflag.FlagSet, f *gnuflag.FlagSet) []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "Usage: %s", i.Name)
	hasOptions := false
	f.VisitAll(func(f *gnuflag.Flag) { hasOptions = true })
	if hasOptions {
		fmt.Fprintf(buf, " [%vs]", f.FlagKnownAs)
	}
	if i.Args != "" {
		fmt.Fprintf(buf, " %s", i.Args)
	}
	fmt.Fprintf(buf, "\n")
	if i.Purpose != "" {
		fmt.Fprintf(buf, "\nSummary:\n%s\n", strings.TrimSpace(i.Purpose))
	}
	hasSuperFlags := false
	if superF != nil && len(i.ShowSuperFlags) != 0 {
		filteredSuperF := gnuflag.NewFlagSetWithFlagKnownAs("", gnuflag.ContinueOnError, superF.FlagKnownAs)
		contains := func(one string) bool {
			for _, a := range i.ShowSuperFlags {
				if strings.EqualFold(one, a) {
					return true
				}
			}
			return false
		}
		superF.VisitAll(func(flag *gnuflag.Flag) {
			if contains(flag.Name) {
				hasSuperFlags = true
				filteredSuperF.Var(flag.Value, flag.Name, flag.Usage)
			}
		})
		if hasSuperFlags {
			fmt.Fprintf(buf, "\nGlobal %vs:\n", strings.Title(filteredSuperF.FlagKnownAs))
			filteredSuperF.SetOutput(buf)
			filteredSuperF.PrintDefaults()
			filteredSuperF.SetOutput(ioutil.Discard)
		}
	}

	if hasOptions {
		if hasSuperFlags {
			fmt.Fprintf(buf, "\nCommand %vs:\n", strings.Title(f.FlagKnownAs))
		} else {
			fmt.Fprintf(buf, "\n%vs:\n", strings.Title(f.FlagKnownAs))
		}
		f.SetOutput(buf)
		f.PrintDefaults()
	}
	f.SetOutput(ioutil.Discard)
	if i.Doc != "" {
		fmt.Fprintf(buf, "\nDetails:\n")
		fmt.Fprintf(buf, "%s\n", strings.TrimSpace(i.Doc))
	}
	if len(i.Aliases) > 0 {
		fmt.Fprintf(buf, "\nAliases: %s\n", strings.Join(i.Aliases, ", "))
	}
	if len(i.Examples) > 0 {
		fmt.Fprintf(buf, "\nExamples:\n%s", i.Examples)
	}
	if len(i.Subcommands) > 0 {
		fmt.Fprintf(buf, "\n%s", i.describeCommands())
	}
	if len(i.SeeAlso) > 0 {
		fmt.Fprintf(buf, "\nSee also:\n")
		for _, entry := range i.SeeAlso {
			fmt.Fprintf(buf, " - %s\n", entry)
		}
	}

	return buf.Bytes()
}

// Default commands should be hidden from the help output.
func isDefaultCommand(cmd string) bool {
	switch cmd {
	case "documentation", "help", "version":
		return true
	}
	return false
}

func (i *Info) describeCommands() string {
	// Sort command names, and work out length of the longest one
	cmdNames := make([]string, 0, len(i.Subcommands))
	longest := 0
	for name := range i.Subcommands {
		if isDefaultCommand(name) {
			continue
		}
		if len(name) > longest {
			longest = len(name)
		}
		cmdNames = append(cmdNames, name)
	}
	sort.Strings(cmdNames)

	descr := "Subcommands:\n"
	for _, name := range cmdNames {
		purpose := i.Subcommands[name]
		descr += fmt.Sprintf("    %-*s - %s\n", longest, name, purpose)
	}
	return descr
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
		_, _ = ctx.Stdout.Write(c.Info().Help(f))
		return 0, true
	case ErrSilent:
		return 2, true
	default:
		WriteError(ctx.Stderr, err)
		return 2, true
	}
}

func FlagAlias(c Command, akaDefault string) string {
	flagsAKA := c.Info().FlagKnownAs
	if flagsAKA == "" {
		return akaDefault
	}
	return flagsAKA
}

// Main runs the given Command in the supplied Context with the given
// arguments, which should not include the command name. It returns a code
// suitable for passing to os.Exit.
func Main(c Command, ctx *Context, args []string) int {
	f := gnuflag.NewFlagSetWithFlagKnownAs(c.Info().Name, gnuflag.ContinueOnError, FlagAlias(c, "flag"))
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
		if utils.IsRcPassthroughError(err) {
			return err.(*utils.RcPassthroughError).Code
		}
		if err != ErrSilent {
			WriteError(ctx.Stderr, err)
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
	ctx := &Context{
		Dir:    abs,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	ctx.Context = context.Background()
	return ctx, nil
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
