package control_test

import (
	"flag"
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

func (c *testCommand) Run() error {
	return nil
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
