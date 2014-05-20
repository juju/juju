// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package audit

import (
	"testing"
	"time"

	"github.com/juju/loggo"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type auditSuite struct{}

var _ = gc.Suite(&auditSuite{})

func (*auditSuite) SetUpTest(c *gc.C) {
	loggo.ResetLoggers()
	loggo.ResetWriters()
	err := loggo.ConfigureLoggers(`<root>=ERROR; juju.audit=INFO`)
	c.Assert(err, gc.IsNil)
}

// testLogger implements loggo.Writer.
// The struct members represent the contents of the last line written.
type testLogger struct {
	loggo.Level
	Name      string
	Filename  string
	Line      int
	Timestamp time.Time
	Message   string
}

// Write implements loggo.Writer. Each invocation will replace the conents of
// the fields inside this testLogger implementation with the values logged.
func (l *testLogger) Write(level loggo.Level, name, filename string, line int, timestamp time.Time, message string) {
	l.Level = level
	l.Name = name
	l.Filename = filename
	l.Line = line
	l.Timestamp = timestamp
	l.Message = message
}

func (*auditSuite) TestAuditEventWrittenToAuditLogger(c *gc.C) {
	var w testLogger
	loggo.ReplaceDefaultWriter(&w)

	// state.User is a struct, not an interface so it cannot be mocked
	// easily. The username reported later in the test will be blank.
	var u state.User
	Audit(&u, "donut eaten, %v donut(s) remain", 7)

	c.Check(w.Level, gc.Equals, loggo.INFO)
	c.Check(w.Name, gc.Equals, "juju.audit")
	c.Check(w.Filename, gc.Matches, ".*audit_test.go$") // TODO(dfc) this should be the function which called audit.Audit
	c.Check(w.Line, gc.Not(gc.Equals), 0)
	c.Check(w.Timestamp.IsZero(), gc.Equals, false)
	c.Check(w.Message, gc.Equals, `user "": donut eaten, 7 donut(s) remain`)
}

func (*auditSuite) TestAuditWithNilUserWillPanic(c *gc.C) {
	f := func() { Audit(nil, "should never be written") }
	c.Assert(f, gc.PanicMatches, "user cannot be nil")
}
