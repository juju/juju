package main_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/~rogpeppe/juju/gnuflag/flag"
)

type TestCommand struct {
	Name  string
	Value string
}

var _ main.Command = (*TestCommand)(nil)

func (c *TestCommand) Info() *main.Info {
	return &main.Info{
		c.Name,
		"blah usage",
		fmt.Sprintf("%s the juju", c.Name),
		"blah doc",
	}
}

func (c *TestCommand) InitFlagSet(f *flag.FlagSet) {
	f.StringVar(&c.Value, "value", "", "doc")
}

func (c *TestCommand) ParsePositional(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("BADARGS: %s", args)
	}
	return nil
}

func (c *TestCommand) Run() error {
	return fmt.Errorf("BORKEN: value is %s.", c.Value)
}

func parseEmpty(args []string) (*main.JujuCommand, error) {
	jc := main.NewJujuCommand()
	err := main.Parse(jc, false, args)
	return jc, err
}

func parseDefenestrate(args []string) (*main.JujuCommand, *TestCommand, error) {
	jc := main.NewJujuCommand()
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	err := main.Parse(jc, false, args)
	return jc, tc, err
}

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

func (s *CommandSuite) TestSubcommandDispatch(c *C) {
	_, err := parseEmpty([]string{})
	c.Assert(err, ErrorMatches, `no command specified`)

	_, _, err = parseDefenestrate([]string{"discombobulate"})
	c.Assert(err, ErrorMatches, "unrecognised command: discombobulate")

	_, tc, err := parseDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "")

	_, tc, err = parseDefenestrate([]string{"defenestrate", "--value", "firmly"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "firmly")

	_, tc, err = parseDefenestrate([]string{"defenestrate", "gibberish"})
	c.Assert(err, ErrorMatches, `BADARGS: \[gibberish\]`)
}

func (s *CommandSuite) TestRegister(c *C) {
	jc := main.NewJujuCommand()
	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flap"})

	badCall := func() { jc.Register(&TestCommand{Name: "flap"}) }
	c.Assert(badCall, PanicMatches, "command already registered: flap")

	cmds := jc.DescribeCommands()
	c.Assert(cmds, Equals, "flap\n    flap the juju\nflip\n    flip the juju\n")
}

func (s *CommandSuite) TestVerbose(c *C) {
	jc, err := parseEmpty([]string{})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Verbose, Equals, false)

	jc, _, err = parseDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose, Equals, false)

	jc, err = parseEmpty([]string{"--verbose"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Verbose, Equals, true)

	jc, _, err = parseDefenestrate([]string{"-v", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose, Equals, true)
}

func (s *CommandSuite) TestLogfile(c *C) {
	jc, err := parseEmpty([]string{})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Logfile, Equals, "")

	jc, _, err = parseDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile, Equals, "")

	jc, err = parseEmpty([]string{"-l", "foo"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Logfile, Equals, "foo")

	jc, _, err = parseDefenestrate([]string{"--log-file", "bar", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile, Equals, "bar")
}

func (s *CommandSuite) TestRun(c *C) {
	jc, _, err := parseDefenestrate([]string{"defenestrate", "--value", "cheese"})
	c.Assert(err, IsNil)

	err = jc.Run()
	c.Assert(err, ErrorMatches, "BORKEN: value is cheese.")
}
