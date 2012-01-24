package main_test

import (
	"flag"
	"fmt"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
)

type testCommand struct {
	value string
}

func (c *testCommand) Parse(args []string) error {
	fs := flag.NewFlagSet("defenestrate", flag.ContinueOnError)
	fs.StringVar(&c.value, "value", "", "doc")
	return fs.Parse(args)
}

func (c *testCommand) PrintUsage() {}

func (c *testCommand) Run() error {
	return fmt.Errorf("This doesn't work, but value is %s.", c.value)
}

func parseEmpty(args []string) (*main.JujuCommand, error) {
	jc := main.NewJujuCommand()
	err := jc.Parse(args)
	return jc, err
}

func parseDefenestrate(args []string) (*main.JujuCommand, *testCommand, error) {
	jc := main.NewJujuCommand()
	tc := new(testCommand)
	jc.Register("defenestrate", tc)
	err := jc.Parse(args)
	return jc, tc, err
}

func (s *CommandSuite) TestSubcommandDispatch(c *C) {
	_, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, `no subcommand specified`)

	_, _, err = parseDefenestrate([]string{"juju", "discombobulate"})
	c.Assert(err, ErrorMatches, `no discombobulate subcommand registered`)

	_, tc, err := parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(tc.value, Equals, "")

	_, tc, err = parseDefenestrate([]string{"juju", "defenestrate", "--value", "firmly"})
	c.Assert(err, IsNil)
	c.Assert(tc.value, Equals, "firmly")

	_, tc, err = parseDefenestrate([]string{"juju", "defenestrate", "--gibberish", "burble"})
	c.Assert(err, ErrorMatches, "flag provided but not defined: -gibberish")
}

func (s *CommandSuite) TestRegister(c *C) {
	jc := main.NewJujuCommand()
	err := jc.Register("flip", new(testCommand))
	c.Assert(err, IsNil)

	err = jc.Register("flop", new(testCommand))
	c.Assert(err, IsNil)

	err = jc.Register("flop", new(testCommand))
	c.Assert(err, ErrorMatches, "subcommand flop is already registered")
}

func (s *CommandSuite) TestVerbose(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	c.Assert(jc.Verbose(), Equals, false)

	jc, _, err = parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose(), Equals, false)

	jc, err = parseEmpty([]string{"juju", "--verbose"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	c.Assert(jc.Verbose(), Equals, true)

	jc, _, err = parseDefenestrate([]string{"juju", "-v", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose(), Equals, true)
}

func (s *CommandSuite) TestLogfile(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	c.Assert(jc.Logfile(), Equals, "")

	jc, _, err = parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile(), Equals, "")

	jc, err = parseEmpty([]string{"juju", "-l", "foo"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	c.Assert(jc.Logfile(), Equals, "foo")

	jc, _, err = parseDefenestrate([]string{"juju", "--log-file", "bar", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile(), Equals, "bar")
}

func (s *CommandSuite) TestRun(c *C) {
	jc, _, err := parseDefenestrate([]string{"juju", "defenestrate", "--value", "cheese"})
	c.Assert(err, IsNil)

	err = jc.Run()
	c.Assert(err, ErrorMatches, "This doesn't work, but value is cheese.")
}

func (s *CommandSuite) TestRunBadParse(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	err = jc.Run()
	c.Assert(err, ErrorMatches, "no subcommand selected")
}
