package cmd_test

import (
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/log"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type TestCommand struct {
	Name  string
	Value string
}

func (c *TestCommand) Info() *cmd.Info {
	return &cmd.Info{c.Name, "<something>", c.Name + " the juju", c.Name + " doc"}
}

func (c *TestCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.StringVar(&c.Value, "value", "", "doc")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *TestCommand) Run(ctx *cmd.Context) error {
	log.Debugf(c.Value)
	return nil
}

func initCmd(c cmd.Command, args []string) error {
	return c.Init(gnuflag.NewFlagSet("", gnuflag.ContinueOnError), args)
}

func initEmpty(args []string) (*cmd.SuperCommand, error) {
	jc := cmd.NewSuperCommand("jujutest", "", "")
	return jc, initCmd(jc, args)
}

func initDefenestrate(args []string) (*cmd.SuperCommand, *TestCommand, error) {
	jc := cmd.NewSuperCommand("jujutest", "", "")
	tc := &TestCommand{Name: "defenestrate"}
	jc.Register(tc)
	return jc, tc, initCmd(jc, args)
}

type CommandSuite struct{}

var _ = Suite(&CommandSuite{})

func (s *CommandSuite) TestDispatch(c *C) {
	jc, err := initEmpty([]string{})
	c.Assert(err, ErrorMatches, `no command specified`)
	info := jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Equals, "")

	jc, _, err = initDefenestrate([]string{"discombobulate"})
	c.Assert(err, ErrorMatches, "unrecognised command: jujutest discombobulate")
	info = jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Args, Equals, "<command> ...")
	c.Assert(info.Doc, Equals, "commands:\n    defenestrate defenestrate the juju")

	jc, tc, err := initDefenestrate([]string{"defenestrate"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "")
	info = jc.Info()
	c.Assert(info.Name, Equals, "jujutest defenestrate")
	c.Assert(info.Args, Equals, "<something>")
	c.Assert(info.Doc, Equals, "defenestrate doc")

	_, tc, err = initDefenestrate([]string{"defenestrate", "--value", "firmly"})
	c.Assert(err, IsNil)
	c.Assert(tc.Value, Equals, "firmly")

	_, tc, err = initDefenestrate([]string{"defenestrate", "gibberish"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[gibberish\]`)
}

func (s *CommandSuite) TestSubcommands(c *C) {
	jc := cmd.NewSuperCommand("jujutest", "to be purposeful", "doc\nblah\ndoc")
	jc.Register(&TestCommand{Name: "flip"})
	jc.Register(&TestCommand{Name: "flap"})
	badCall := func() { jc.Register(&TestCommand{Name: "flap"}) }
	c.Assert(badCall, PanicMatches, "command already registered: flap")

	info := jc.Info()
	c.Assert(info.Name, Equals, "jujutest")
	c.Assert(info.Purpose, Equals, "to be purposeful")
	c.Assert(info.Doc, Equals, `doc
blah
doc

commands:
    flap         flap the juju
    flip         flip the juju`)
}

func (s *CommandSuite) TestLogging(c *C) {
	defer saveLog()()
	jc := cmd.NewLoggingSuperCommand("jujutest", "", "")
	jc.Register(&TestCommand{Name: "blah"})
	ctx := dummyContext(c)
	code := cmd.Main(jc, ctx, []string{"blah", "--value", "arrived", "--debug"})
	c.Assert(code, Equals, 0)
	c.Assert(str(ctx.Stderr), Matches, `.* JUJU:DEBUG arrived\n`)
}
