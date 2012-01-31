package main

import (
	"fmt"
	"io"
	"launchpad.net/juju/go/log"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
	stdlog "log"
	"os"
	"sort"
	"strings"
)

var (
	cmdTemplate = "%s\n    %s\n"
	docTemplate = `
juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

https://juju.ubuntu.com/

commands:
%s`
)

type JujuCommand struct {
	Logfile string
	Verbose bool
	Debug   bool
	subcmds map[string]Command
	subcmd  Command
}

// Ensure Command interface.
var _ Command = (*JujuCommand)(nil)

// NewJujuCommand returns an initialized JujuCommand.
func NewJujuCommand() *JujuCommand {
	return &JujuCommand{subcmds: make(map[string]Command)}
}

// Register makes a subcommand available for use on the command line.
func (c *JujuCommand) Register(subcmd Command) {
	name := subcmd.Info().Name
	_, alreadythere := c.subcmds[name]
	if alreadythere {
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
		cmds[i] = fmt.Sprintf(cmdTemplate, name, purpose)
	}
	return strings.Join(cmds, "")
}

// Info returns a description of the currently selected subcommand, or of the
// juju command itself if no subcommand has been specified.
func (c *JujuCommand) Info() *Info {
	if c.subcmd != nil {
		return c.subcmd.Info()
	}
	return &Info{
		"juju",
		"juju <command> [options] ...",
		"",
		fmt.Sprintf(docTemplate, c.DescribeCommands()),
	}
}

// InitFlagSet prepares a FlagSet for use with the currently selected
// subcommand, or with the juju command itself if no subcommand has been
// specified.
func (c *JujuCommand) InitFlagSet(f *flag.FlagSet) {
	if c.subcmd != nil {
		c.subcmd.InitFlagSet(f)
	}
	// All subcommands should also be expected to accept these options
	f.StringVar(&c.Logfile, "logfile", c.Logfile, "path to write log to")
	f.BoolVar(&c.Verbose, "v", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Verbose, "verbose", c.Verbose, "if set, log additional messages")
	f.BoolVar(&c.Debug, "d", c.Debug, "if set, log many additional messages")
	f.BoolVar(&c.Debug, "debug", c.Debug, "if set, log many additional messages")
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

// initOutput sets up logging to a file, or to stderr, depending on what's been
// requested on the command line.
func (c *JujuCommand) initOutput() error {
	if c.Debug {
		log.Debug = true
	}
	var target io.Writer
	if c.Logfile != "" {
		var err error
		target, err = os.OpenFile(c.Logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
	} else if c.Verbose || c.Debug {
		target = os.Stderr
	}
	if target != nil {
		log.Target = stdlog.New(target, "", 0)
	}
	return nil
}

// Run executes the selected subcommand, which depends on Parse having been
// called with the JujuCommand.
func (c *JujuCommand) Run() error {
	if err := c.initOutput(); err != nil {
		return err
	}
	return c.subcmd.Run()
}
