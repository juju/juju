package testing

import (
	"bytes"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

// NewFlagSet creates a new flag set using the standard options, particularly
// the option to stop the gnuflag methods from writing to StdErr or StdOut.
func NewFlagSet() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	return fs
}

// InitCommand will create a new flag set, and call the Command's SetFlags and
// Init methods with the appropriate args.
func InitCommand(c cmd.Command, args []string) error {
	f := NewFlagSet()
	c.SetFlags(f)
	if err := cmd.ParseArgs(c, f, args); err != nil {
		return err
	}
	return c.Init(f.Args())
}

// RunCommand will run a command with the specified args.  The returned error
// may come from either the parsing of the args, the command initialisation or
// the actual running of the command.  Access to the resulting output streams
// is not provided with this function.
func RunCommand(c *C, com cmd.Command, args []string) error {
	if err := InitCommand(com, args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

// TestInit checks that a command initialises correctly with the given set of
// arguments.
func TestInit(c *C, com cmd.Command, args []string, errPat string) {
	err := InitCommand(com, args)
	if errPat != "" {
		c.Assert(err, ErrorMatches, errPat)
	} else {
		c.Assert(err, IsNil)
	}
}
