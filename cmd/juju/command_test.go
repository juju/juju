package main_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
)

func parseEmpty(args []string) (*main.JujuCommand, error) {
	jc := &main.JujuCommand{}
	err := jc.Parse(args)
	return jc, err
}

func parseDefenestrate(args []string) (*main.JujuCommand, *main.TestCommand, error) {
	jc := &main.JujuCommand{}
	tc := &main.TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	err := jc.Parse(args)
	return jc, tc, err
}

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

func (s *CommandSuite) TestSubcommandDispatch(c *C) {
	_, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, `no command specified`)

	_, _, err = parseDefenestrate([]string{"juju", "discombobulate"})
	c.Assert(err, ErrorMatches, "unrecognised command: discombobulate")

	_, tc, err := parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "")

	_, tc, err = parseDefenestrate([]string{"juju", "defenestrate", "--value", "firmly"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "firmly")

	_, tc, err = parseDefenestrate([]string{"juju", "defenestrate", "--gibberish", "burble"})
	c.Assert(err, ErrorMatches, "flag provided but not defined: --gibberish")
}

func (s *CommandSuite) TestRegister(c *C) {
	jc := &main.JujuCommand{}
	err := jc.Register(&main.TestCommand{Name: "flip"})
	c.Assert(err, IsNil)

	err = jc.Register(&main.TestCommand{Name: "flap"})
	c.Assert(err, IsNil)

	err = jc.Register(&main.TestCommand{Name: "flap"})
	c.Assert(err, ErrorMatches, "command already registered: flap")

	buf := &bytes.Buffer{}
	jc.DescCommands(buf)
	c.Assert(buf.String(), Equals, "\ncommands:\nflap\n    command named flap\nflip\n    command named flip\n")
}

func (s *CommandSuite) TestVerbose(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Verbose(), Equals, false)

	jc, _, err = parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose(), Equals, false)

	jc, err = parseEmpty([]string{"juju", "--verbose"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Verbose(), Equals, true)

	jc, _, err = parseDefenestrate([]string{"juju", "-v", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Verbose(), Equals, true)
}

func (s *CommandSuite) TestLogfile(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Logfile(), Equals, "")

	jc, _, err = parseDefenestrate([]string{"juju", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile(), Equals, "")

	jc, err = parseEmpty([]string{"juju", "-l", "foo"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Logfile(), Equals, "foo")

	jc, _, err = parseDefenestrate([]string{"juju", "--log-file", "bar", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Logfile(), Equals, "bar")
}

func (s *CommandSuite) TestRun(c *C) {
	jc, _, err := parseDefenestrate([]string{"juju", "defenestrate", "--value", "cheese"})
	c.Assert(err, IsNil)

	err = jc.Run()
	c.Assert(err, ErrorMatches, "BORKEN: value is cheese.")
}

func (s *CommandSuite) TestRunBadParse(c *C) {
	jc, err := parseEmpty([]string{"juju"})
	c.Assert(err, ErrorMatches, "no command specified")
	err = jc.Run()
	c.Assert(err, ErrorMatches, "no command selected")
}
