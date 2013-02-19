package testing

import (
	"bytes"
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

func NewFlagSet() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	return fs
}

func InitCommand(c cmd.Command, args []string) error {
	f := NewFlagSet()
	c.SetFlags(f)
	if err := cmd.ParseArgs(c, f, args); err != nil {
		return err
	}
	return c.Init(f.Args())
}

func RunCommand(c *C, com cmd.Command, args []string) error {
	if err := InitCommand(com, args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

// testInit checks that a command initialises correctly
// with the given set of arguments.
func TestInit(c *C, com cmd.Command, args []string, errPat string) {
	err := InitCommand(com, args)
	if errPat != "" {
		c.Assert(err, ErrorMatches, errPat)
	} else {
		c.Assert(err, IsNil)
	}
}
