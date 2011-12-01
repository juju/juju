package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	jujulog "launchpad.net/juju/go/log"
	"log"
	"testing"
)

const (
	logPrefix = "JUJU "
	dbgPrefix = "JUJU:DEBUG "
)

func Test(t *testing.T) {
	TestingT(t)
}

type suite struct{}

var _ = Suite(suite{})

type logTest struct {
	input string
	debug bool
}

var logTests = []struct {
	input string
	debug bool
}{
	{
		input: "Hello World",
		debug: false,
	},
	{
		input: "Hello World",
		debug: true,
	},
}

func (suite) TestLogger(c *C) {
	buf := &bytes.Buffer{}
	jujulog.GlobalLogger = log.New(buf, "", 0)
	for _, t := range logTests {
		jujulog.Debug = t.debug
		jujulog.Logf(t.input)
		c.Assert(buf.String(), Equals, logPrefix+t.input+"\n")
		buf.Reset()
		jujulog.Debugf(t.input)
		if t.debug {
			c.Assert(buf.String(), Equals, dbgPrefix+t.input+"\n")
		} else {
			c.Assert(buf.String(), Equals, "")
		}
		buf.Reset()
	}
}
