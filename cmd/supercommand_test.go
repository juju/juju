package cmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/testing"
)

func initDefenestrate(args []string) (*cmd.SuperCommand, *TestCommand, error) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	return jc, tc, testing.InitCommand(jc, args)
}

type SuperCommandSuite struct{}

var _ = Suite(&SuperCommandSuite{})

const helpText = "\n    help\\s+- show help on a command or other topic"
const helpCommandsText = "commands:" + helpText

func (s *SuperCommandSuite) TestDispatch(c *C) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	info := jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Matches, helpCommandsText)

	jc, _, err := initDefenestrate([]string{"discombobulate"})
	c.Assert(err, ErrorMatches, "unrecognized command: jujutest discombobulate")
	info = jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Matches, "commands:\n    defenestrate - defenestrate the juju"+helpText)

	jc, tc, err := initDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(tc.Option, Equals, "")
	info = jc.Info()
	c.Assert(info.Name, Equals, "jujutest defenestrate")
	c.Assert(info.Args, Equals, "<something>")
	c.Assert(info.Doc, Equals, "defenestrate-doc")

	_, tc, err = initDefenestrate([]string{"defenestrate", "--option", "firmly"})
	c.Assert(err, IsNil)
	c.Assert(tc.Option, Equals, "firmly")

	_, tc, err = initDefenestrate([]string{"defenestrate", "gibberish"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["gibberish"\]`)
}

func (s *SuperCommandSuite) TestRegister(c *C) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flap"})
	badCall := func() { jc.Register(&TestCommand{Name: "flap"}) }
	c.Assert(badCall, PanicMatches, "command already registered: flap")
}

func (s *SuperCommandSuite) TestRegisterAlias(c *C) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	jc.Register(&TestCommand{Name: "flip", Aliases: []string{"flap", "flop"}})

	info := jc.Info()
	c.Assert(info.Doc, Equals, `commands:
    flap - alias for flip
    flip - flip the juju
    flop - alias for flip
    help - show help on a command or other topic`)
}

var commandsDoc = `commands:
    flapbabble - flapbabble the juju
    flip       - flip the juju`

func (s *SuperCommandSuite) TestInfo(c *C) {
	jc := &cmd.SuperCommand{
		Name: "jujutest", Purpose: "to be purposeful", Doc: "doc\nblah\ndoc",
	}
	info := jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Purpose, Equals, "to be purposeful")
	// info doc starts with the jc.Doc and ends with the help command
	c.Assert(info.Doc, Matches, jc.Doc+"(.|\n)*")
	c.Assert(info.Doc, Matches, "(.|\n)*"+helpCommandsText)

	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flapbabble"})
	info = jc.Info()
	c.Assert(info.Doc, Matches, jc.Doc+"\n\n"+commandsDoc+helpText)

	jc.Doc = ""
	info = jc.Info()
	c.Assert(info.Doc, Matches, commandsDoc+helpText)
}

func (s *SuperCommandSuite) TestLogging(c *C) {
	target, debug := log.Target, log.Debug
	defer func() {
		log.Target, log.Debug = target, debug
	}()
	jc := &cmd.SuperCommand{Name: "jujutest", Log: &cmd.Log{}}
	jc.Register(&TestCommand{Name: "blah"})
	ctx := testing.Context(c)
	code := cmd.Main(jc, ctx, []string{"blah", "--option", "error", "--debug"})
	c.Assert(code, Equals, 1)
	c.Assert(bufferString(ctx.Stderr), Matches, `.* JUJU jujutest blah command failed: BAM!
error: BAM!
`)
}
