package cmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/log"
)

func initDefenestrate(args []string) (*cmd.SuperCommand, *TestCommand, error) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	return jc, tc, jc.Init(dummyFlagSet(), args)
}

type SuperCommandSuite struct{}

var _ = Suite(&SuperCommandSuite{})

func (s *SuperCommandSuite) TestDispatch(c *C) {
	jc := &cmd.SuperCommand{Name: "jujutest"}
	err := jc.Init(dummyFlagSet(), []string{})
	c.Assert(err, ErrorMatches, `no command specified`)
	info := jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Equals, "")

	jc, _, err = initDefenestrate([]string{"discombobulate"})
	c.Assert(err, ErrorMatches, "unrecognized command: jujutest discombobulate")
	info = jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Equals, "commands:\n    defenestrate - defenestrate the juju")

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
	c.Assert(info.Doc, Equals, jc.Doc)

	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flapbabble"})
	info = jc.Info()
	c.Assert(info.Doc, Equals, jc.Doc+"\n\n"+commandsDoc)

	jc.Doc = ""
	info = jc.Info()
	c.Assert(info.Doc, Equals, commandsDoc)
}

func (s *SuperCommandSuite) TestLogging(c *C) {
	target, debug := log.Target, log.Debug
	defer func() {
		log.Target, log.Debug = target, debug
	}()
	jc := &cmd.SuperCommand{Name: "jujutest", Log: &cmd.Log{}}
	jc.Register(&TestCommand{Name: "blah"})
	ctx := dummyContext(c)
	code := cmd.Main(jc, ctx, []string{"blah", "--option", "error", "--debug"})
	c.Assert(code, Equals, 1)
	c.Assert(str(ctx.Stderr), Matches, `.* JUJU:DEBUG jujutest blah command failed: BAM!
error: BAM!
`)
}
