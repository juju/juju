package control_test

import (
	"flag"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/control"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

type testCommand struct {
	value string
}

func (c *testCommand) Parse(args []string) error {
	fs := flag.NewFlagSet("defenestrate", flag.ContinueOnError)
	fs.StringVar(&c.value, "value", "", "doc")
	return fs.Parse(args)
}

func (c *testCommand) Usage() string {
	return "Crash bang wallop."
}

func (c *testCommand) Run() error {
	return fmt.Errorf("This doesn't work, but value is %s.", c.value)
}

func parseEmpty(args []string) (*control.JujuCommand, error) {
	jc := new(control.JujuCommand)
	err := jc.Parse(args)
	return jc, err
}

func parseDefenestrate(args []string) (*control.JujuCommand, *testCommand, error) {
	jc := new(control.JujuCommand)
	tc := new(testCommand)
	jc.Register("defenestrate", tc)
	err := jc.Parse(args)
	return jc, tc, err
}

func (s *CommandSuite) TestSubcommandDispatch(c *C) {
	_, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, `no subcommand specified`)

	_, err = parseEmpty([]string{"juju", "defenstrate"})
	c.Assert(err, ErrorMatches, `no subcommands registered`)

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
	jc := new(control.JujuCommand)
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

func (s *CommandSuite) TestUsage(c *C) {
	// Yes, this stuff will need to change.
	jc, _, _ := parseDefenestrate([]string{"juju"})
	c.Assert(jc.Usage(), Equals, "You're Doing It Wrong.")

	jc, _, _ = parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(jc.Usage(), Equals, "Crash bang wallop.")
}

func (s *CommandSuite) TestRunBadParse(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no subcommand specified")
	err = jc.Run()
	c.Assert(err, ErrorMatches, "no subcommand selected")
}

func (s *CommandSuite) TestJujuMainCommand(c *C) {
	jmc := control.JujuMainCommand()
	// Precise errors are not interesting here, this is a very high-level test
	assertParses := func(args []string) { c.Assert(jmc.Parse(args), IsNil) }
	assertErrors := func(args []string) { c.Assert(jmc.Parse(args), NotNil) }

	assertErrors([]string{"juju"})
	assertErrors([]string{"juju", "-v"})
	assertErrors([]string{"juju", "-l"})
	assertErrors([]string{"juju", "-l", "some.log"})
	assertErrors([]string{"juju", "twiddle"})
	assertErrors([]string{"juju", "-v", "twiddle"})
	assertErrors([]string{"juju", "-l", "some.log", "twiddle"})

	assertParses([]string{"juju", "bootstrap"})
	assertParses([]string{"juju", "-v", "bootstrap"})
	assertParses([]string{"juju", "-l", "some.log", "bootstrap"})
	assertParses([]string{"juju", "bootstrap", "-e", "env"})
	assertParses([]string{"juju", "-v", "bootstrap", "-e", "env"})
	assertParses([]string{"juju", "-l", "some.log", "bootstrap", "-e", "env"})

	assertErrors([]string{"juju", "bootstrap", "-v"})
	assertErrors([]string{"juju", "bootstrap", "-l", "some.log"})
}
