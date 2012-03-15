package hook_test

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/hook"
	"launchpad.net/juju/go/log"
	stdlog "log"
)

type ContextSuite struct{}

var _ = Suite(&ContextSuite{})

func saveLog(debug bool) (*bytes.Buffer, func()) {
	oldTarget, oldDebug := log.Target, log.Debug
	buf := bytes.NewBuffer([]byte{})
	log.Target, log.Debug = stdlog.New(buf, "", 0), debug
	return buf, func() {
		log.Target, log.Debug = oldTarget, oldDebug
	}
}

func AssertLog(c *C, ctx *hook.Context, badge string, logDebug, callDebug, expectMsg bool) {
	buf, restore := saveLog(logDebug)
	defer restore()
	msg := "the chickens are restless"
	ctx.Log(callDebug, msg)
	expect := ""
	if expectMsg {
		var logBadge string
		if callDebug {
			logBadge = "JUJU:DEBUG"
		} else {
			logBadge = "JUJU"
		}
		expect = fmt.Sprintf("%s %s: %s\n", logBadge, badge, msg)
	}
	c.Assert(buf.String(), Equals, expect)
}

func AssertLogs(c *C, ctx *hook.Context, badge string) {
	AssertLog(c, ctx, badge, true, true, true)
	AssertLog(c, ctx, badge, true, false, true)
	AssertLog(c, ctx, badge, false, true, false)
	AssertLog(c, ctx, badge, false, false, true)
}

// statelessContexts is useful for testing the Context methods that don't need
// to touch state, and can therefore be called without one.
func statelessContexts() (*hook.Context, *hook.Context, *hook.Context) {
	return hook.NewLocalContext(nil, "minecraft/0"),
		hook.NewRelationContext(nil, "minecraft/0", "bot", []string{"bot/0"}),
		hook.NewBrokenContext(nil, "minecraft/0", "bot")
}

func (s *ContextSuite) TestLog(c *C) {
	local, relation, broken := statelessContexts()
	AssertLogs(c, local, "Context<minecraft/0>")
	AssertLogs(c, relation, "Context<minecraft/0, bot>")
	AssertLogs(c, broken, "Context<minecraft/0, bot>")
}

func (s *ContextSuite) TestMembers(c *C) {
	local, relation, broken := statelessContexts()
	c.Assert(local.Members(), DeepEquals, []string{})
	c.Assert(relation.Members(), DeepEquals, []string{"bot/0"})
	c.Assert(broken.Members(), DeepEquals, []string{})
}

func (s *ContextSuite) TestVars(c *C) {
	local, relation, broken := statelessContexts()
	possibles := []string{"JUJU_UNIT_NAME=minecraft/0", "JUJU_RELATION=bot"}
	c.Assert(local.Vars(), DeepEquals, possibles[:1])
	c.Assert(relation.Vars(), DeepEquals, possibles)
	c.Assert(broken.Vars(), DeepEquals, possibles)
}
