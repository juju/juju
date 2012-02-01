package main_test

import (
	"fmt"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
	"launchpad.net/juju/go/log"
	"os"
	"path/filepath"
)

type TestCommand struct {
	Name  string
	Value string
}

func (c *TestCommand) Info() *main.Info {
	return &main.Info{
		c.Name,
		"blah usage",
		fmt.Sprintf("%s the juju", c.Name),
		"blah doc",
	}
}

func (c *TestCommand) InitFlagSet(f *gnuflag.FlagSet) {
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
	c.Assert(cmds, Equals, "    flap         flap the juju\n    flip         flip the juju\n")
}

func (s *CommandSuite) TestDebug(c *C) {
	jc, err := parseEmpty([]string{})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Debug, Equals, false)

	jc, _, err = parseDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Debug, Equals, false)

	jc, err = parseEmpty([]string{"--debug"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.Debug, Equals, true)

	jc, _, err = parseDefenestrate([]string{"-d", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.Debug, Equals, true)
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

func (s *CommandSuite) TestLogFile(c *C) {
	jc, err := parseEmpty([]string{})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.LogFile, Equals, "")

	jc, _, err = parseDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.LogFile, Equals, "")

	jc, err = parseEmpty([]string{"--log-file", "foo"})
	c.Assert(err, ErrorMatches, "no command specified")
	c.Assert(jc.LogFile, Equals, "foo")

	jc, _, err = parseDefenestrate([]string{"--log-file", "bar", "defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(jc.LogFile, Equals, "bar")
}

func saveLog() func() {
	target, debug := log.Target, log.Debug
	log.Target, log.Debug = nil, false
	return func() {
		log.Target, log.Debug = target, debug
	}
}

func checkRun(c *C, args []string, debug bool, target Checker, logfile string) {
	defer saveLog()()
	args = append([]string{"defenestrate", "--value", "cheese"}, args...)
	jc, _, err := parseDefenestrate(args)
	c.Assert(err, IsNil)

	err = jc.Run()
	c.Assert(err, ErrorMatches, "BORKEN: value is cheese.")

	c.Assert(log.Debug, Equals, debug)
	c.Assert(log.Target, target)
	if logfile != "" {
		_, err := os.Stat(logfile)
		c.Assert(err, IsNil)
	}
}

func (s *CommandSuite) TestRun(c *C) {
	checkRun(c, []string{}, false, IsNil, "")
	checkRun(c, []string{"--debug"}, true, NotNil, "")
	checkRun(c, []string{"--verbose"}, false, NotNil, "")
	checkRun(c, []string{"--verbose", "--debug"}, true, NotNil, "")

	tmp := c.MkDir()
	path := filepath.Join(tmp, "log-1")
	checkRun(c, []string{"--log-file", path}, false, NotNil, path)

	path = filepath.Join(tmp, "log-2")
	checkRun(c, []string{"--log-file", path, "--debug"}, true, NotNil, path)

	path = filepath.Join(tmp, "log-3")
	checkRun(c, []string{"--log-file", path, "--verbose"}, false, NotNil, path)

	path = filepath.Join(tmp, "log-4")
	checkRun(c, []string{"--log-file", path, "--verbose", "--debug"}, true, NotNil, path)
}
