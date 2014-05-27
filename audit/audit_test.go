// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"testing"

	"github.com/juju/loggo"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type auditSuite struct{}

var _ = gc.Suite(&auditSuite{})

func (*auditSuite) SetUpTest(c *gc.C) {
	loggo.ResetLoggers()
	loggo.ResetWriters()
	err := loggo.ConfigureLoggers(`<root>=ERROR; audit=INFO`)
	c.Assert(err, gc.IsNil)
}

type mockUser struct {
	tag string
}

func (u *mockUser) Tag() string { return u.tag }

func (*auditSuite) TestAuditEventWrittenToAuditLogger(c *gc.C) {
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("audit-log", &tw, loggo.DEBUG), gc.IsNil)

	u := &mockUser{tag: "user-agnus"}
	Audit(u, "donut eaten, %v donut(s) remain", 7)

	// Add deprecated message to be checked.
	messages := []jc.SimpleMessage{
		{loggo.INFO, `user-agnus: donut eaten, 7 donut\(s\) remain`},
	}

	c.Check(tw.Log, jc.LogMatches, messages)
}

func (*auditSuite) TestAuditWithNilUserWillPanic(c *gc.C) {
	f := func() { Audit(nil, "should never be written") }
	c.Assert(f, gc.PanicMatches, "user cannot be nil")
}

func (*auditSuite) TestAuditWithEmptyTagWillPanic(c *gc.C) {
	f := func() { Audit(&mockUser{}, "should never be written") }
	c.Assert(f, gc.PanicMatches, "user tag cannot be blank")
}
