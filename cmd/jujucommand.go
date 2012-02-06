package cmd

import (
	"fmt"
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/log"
	stdlog "log"
	"os"
	"sort"
	"strings"
)

// JujuCommand is a Command that selects a subcommand when Parse is first
// called, and takes on the properties of that subcommand before calling Parse
// again on itself, passing in any remaining command line arguments. Info,
// InitFlagSet, and ParsePositional all dispatch to the selected subcommand
// when appropriate; this is especially important in the case of InitFlagSet,
// because it gives the JujuCommand an opportunity to inject its own flag
// handlers into the command's FlagSet (thereby allowing a natural `juju
// bootstrap -v -e foo` usage style, as opposed to forcing `juju -v bootstrap
// -e foo` (or complicating the code by causing (sub-)Commands to have some
// concept of "parent" Commands).
type JujuCommand struct {
	name    string
	doc     string
	LogFile string
	Verbose bool
	Debug   bool
	subcmds map[string]Command
	subcmd  Command
}

// NewJujuCommand returns an initialized JujuCommand.
func NewJujuCommand(name string, doc string) *JujuCommand {
	return &JujuCommand{
		subcmds: make(map[string]Command),
		name:    name,
		doc:     doc,
	}
}

// Register makes a subcommand available for use on the command line.
func (c *JujuCommand) Register(subcmd Command) {
	name := subcmd.Info().Name
	_, found := c.subcmds[name]
	if found {
		panic(fmt.Sprintf("command already registered: %s", name))
	}
	c.subcmds[name] = subcmd
}

// DescribeCommands returns a short description of each registered subcommand.
func (c *JujuCommand) DescribeCommands() string {
	cmds := make([]string, len(c.subcmds))
	i := 0
	for name, _ := range c.subcmds {
		cmds[i] = name
		i++
	}
	sort.Strings(cmds)
	for i, name := range cmds {
		purpose := c.subcmds[name].Info().Purpose
		cmds[i] = fmt.Sprintf("    %-12s %s\n", name, purpose)
	}
	return fmt.Sprintf("commands:\n%s", strings.Join(cmds, ""))
}

// Info returns a description of the currently selected subcommand, or of the
// JujuCommand itself if no subcommand has been specified.
func (c *JujuCommand) Info() *Info {
	if c.subcmd != nil {
		return c.subcmd.Info()
	}
	return &Info{
		c.name,
		fmt.Sprintf("%s <command> [options] ...", c.name),
		"",
		fmt.Sprintf("%s\n\n%s", strings.TrimSpace(c.doc), c.DescribeCommands()),
	}
}

// InitFlagSet prepares a FlagSet for use with the currently selected
// subcommand, or with the juju command itself if no subcommand has been
// specified.
func (c *JujuCommand) InitFlagSet(f *gnuflag.FlagSet) {
	if c.subcmd != nil {
		c.subcmd.InitFlagSet(f)
	}
	// All subcommands should also be expected to accept these options
	f.StringVar(&c.LogFile, "log-file", c.LogFile, "path to write log to")
	f.BoolVar(&c.Verbose, "v", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Verbose, "verbose", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Debug, "d", c.Debug, "if set, log debugging messages")
	f.BoolVar(&c.Debug, "debug", c.Debug, "if set, log debugging messages")
}

// ParsePositional selects the subcommand specified by subargs and uses it to
// Parse any remaining unconsumed command-line arguments.
func (c *JujuCommand) ParsePositional(subargs []string) error {
	if c.subcmd != nil {
		return c.subcmd.ParsePositional(subargs)
	}
	if len(subargs) == 0 {
		return fmt.Errorf("no command specified")
	}
	found := false
	if c.subcmd, found = c.subcmds[subargs[0]]; !found {
		return fmt.Errorf("unrecognised command: %s", subargs[0])
	}
	return Parse(c, true, subargs[1:])
}

// initOutput sets up logging to a file or to stderr depending on what's been
// requested on the command line.
func (c *JujuCommand) initOutput() error {
	if c.Debug {
		log.Debug = true
	}
	var target io.Writer
	if c.LogFile != "" {
		var err error
		target, err = os.OpenFile(c.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
	} else if c.Verbose || c.Debug {
		target = os.Stderr
	}
	if target != nil {
		log.Target = stdlog.New(target, "", stdlog.LstdFlags)
	}
	return nil
}

// Run executes the subcommand that was selected when Parse was called.
func (c *JujuCommand) Run() error {
	if err := c.initOutput(); err != nil {
		return err
	}
	if c.subcmd == nil {
		panic("Run: missing subcommand; Parse failed or not called")
	}
	return c.subcmd.Run()
}

// Main will Parse and Run a JujuCommand, and exit appropriately.
func Main(jc *JujuCommand, args []string) {
	if err := Parse(jc, false, args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		PrintUsage(jc)
		os.Exit(2)
	}
	if err := jc.Run(); err != nil {
		log.Debugf("%s command failed: %s\n", jc.Info().Name, err)
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
