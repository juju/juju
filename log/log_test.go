package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/log"
	stdlog "log"
	"testing"
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
	log.Target = stdlog.New(buf, "", 0)
	for _, t := range logTests {
		log.Debug = t.debug
		log.Printf(t.input)
		c.Assert(buf.String(), Equals, "JUJU "+t.input+"\n")
		buf.Reset()
		log.Debugf(t.input)
		if t.debug {
			c.Assert(buf.String(), Equals, "JUJU:DEBUG "+t.input+"\n")
		} else {
			c.Assert(buf.String(), Equals, "")
		}
		buf.Reset()
	}
}
