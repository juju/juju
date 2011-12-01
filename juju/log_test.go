package juju_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"log"
)

const logPrefix = "JUJU "
const dbgPrefix = "JUJU:DEBUG "

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
	buf := bytes.NewBuffer(make([]byte, 0))
	l := log.New(buf, "", 0)
	juju.SetLogger(l)
	for _, t := range logTests {
		juju.SetDebug(t.debug)
		juju.Logf(t.input)
		c.Assert(buf.String(), Equals, logPrefix+t.input+"\n")
		buf.Reset()
		juju.Debugf(t.input)
		if t.debug {
			c.Assert(buf.String(), Equals, dbgPrefix+t.input+"\n")
		} else {
			c.Assert(buf.String(), Equals, "")
		}
	}
}
