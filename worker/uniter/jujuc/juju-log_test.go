package jujuc_test

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	stdlog "log"
)

type JujuLogSuite struct {
	ContextSuite
}

var _ = Suite(&JujuLogSuite{})

func pushLog(debug bool) (*bytes.Buffer, func()) {
	oldTarget, oldDebug := log.Target(), log.Debug
	var buf bytes.Buffer
	log.SetTarget(stdlog.New(&buf, "JUJU:", 0))
	log.Debug = debug
	return &buf, func() {
		log.SetTarget(oldTarget)
		log.Debug = oldDebug
	}
}

var commonLogTests = []struct {
	debugEnabled bool
	debugFlag    bool
	target       string
}{
	{false, false, "JUJU:INFO"},
	{false, true, ""},
	{true, false, "JUJU:INFO"},
	{true, true, "JUJU:DEBUG"},
}

func assertLogs(c *C, ctx jujuc.Context, badge string) {
	msg1 := "the chickens"
	msg2 := "are 110% AWESOME"
	com, err := jujuc.NewCommand(ctx, "juju-log")
	c.Assert(err, IsNil)
	for _, t := range commonLogTests {
		buf, pop := pushLog(t.debugEnabled)
		defer pop()

		var args []string
		if t.debugFlag {
			args = []string{"--debug", msg1, msg2}
		} else {
			args = []string{msg1, msg2}
		}
		code := cmd.Main(com, &cmd.Context{}, args)
		c.Assert(code, Equals, 0)

		if t.target == "" {
			c.Assert(buf.String(), Equals, "")
		} else {
			expect := fmt.Sprintf("%s %s: %s %s\n", t.target, badge, msg1, msg2)
			c.Assert(buf.String(), Equals, expect)
		}
	}
}

func (s *JujuLogSuite) TestBadges(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	assertLogs(c, hctx, "u/0")
	hctx = s.GetHookContext(c, 1, "u/1")
	assertLogs(c, hctx, "u/0 peer1:1")
}

func newJujuLogCommand(c *C) cmd.Command {
	ctx := &Context{}
	com, err := jujuc.NewCommand(ctx, "juju-log")
	c.Assert(err, IsNil)
	return com
}

func (s *JujuLogSuite) TestRequiresMessage(c *C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, nil, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l"}, "flag needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l", "FATAL"}, "no message specified")
}

func (s *JujuLogSuite) TestLogDeprecation(c *C) {
	com := newJujuLogCommand(c)
	ctx, err := testing.RunCommand(c, com, []string{"--format", "foo", "msg"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stderr(ctx), Equals, "--format flag deprecated for command \"juju-log\"")
}
