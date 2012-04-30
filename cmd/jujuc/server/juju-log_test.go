package server_test

import (
	"bytes"
	"fmt"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"launchpad.net/juju/go/log"
	stdlog "log"
)

type JujuLogSuite struct{}

var _ = Suite(&JujuLogSuite{})

func pushLog(debug bool) (buf *bytes.Buffer, pop func()) {
	oldTarget, oldDebug := log.Target, log.Debug
	buf = new(bytes.Buffer)
	log.Target, log.Debug = stdlog.New(buf, "", 0), debug
	return buf, func() {
		log.Target, log.Debug = oldTarget, oldDebug
	}
}

func dummyFlagSet() *gnuflag.FlagSet {
	return gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
}

var commonLogTests = []struct {
	debugEnabled bool
	debugFlag    bool
	target       string
}{
	{false, false, "JUJU"},
	{false, true, ""},
	{true, false, "JUJU"},
	{true, true, "JUJU:DEBUG"},
}

func assertLogs(c *C, ctx *server.Context, badge string) {
	msg := "the chickens are 110% AWESOME"
	com, err := ctx.GetCommand("juju-log")
	c.Assert(err, IsNil)
	for _, t := range commonLogTests {
		buf, pop := pushLog(t.debugEnabled)
		defer pop()

		var args []string
		if t.debugFlag {
			args = []string{"--debug", msg}
		} else {
			args = []string{msg}
		}
		code := cmd.Main(com, &cmd.Context{}, args)
		c.Assert(code, Equals, 0)

		if t.target == "" {
			c.Assert(buf.String(), Equals, "")
		} else {
			expect := fmt.Sprintf("%s %s: %s\n", t.target, badge, msg)
			c.Assert(buf.String(), Equals, expect)
		}
	}
}

func (s *JujuLogSuite) TestBadges(c *C) {
	local := &server.Context{LocalUnitName: "minecraft/0"}
	assertLogs(c, local, "minecraft/0")
	relation := &server.Context{LocalUnitName: "minecraft/0", RelationName: "bot"}
	assertLogs(c, relation, "minecraft/0 bot")
}

func (s *JujuLogSuite) TestErrors(c *C) {
	ctx := &server.Context{}
	com, err := ctx.GetCommand("juju-log")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), nil)
	c.Assert(err, ErrorMatches, "no message specified")
	err = com.Init(dummyFlagSet(), []string{"foo", "bar"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[bar\]`)
}
