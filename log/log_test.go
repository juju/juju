package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
	stdlog "log"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type suite struct{}

var _ = Suite(suite{})

func (suite) TestLocalLoggerDebug(c *C) {
	buf := &bytes.Buffer{}
	log.Target = stdlog.New(buf, "JUJU:", 0)
	log.Debug = false
	input := "Hello World"
	log.Debugf(input)
	c.Assert(buf.String(), Equals, "")
	buf.Reset()
	log.Debug = true
	log.Debugf(input)
	c.Assert(buf.String(), Equals, "JUJU:DEBUG: "+input+"\n")
}

func (suite) TestLocalLogger(c *C) {
	buf := &bytes.Buffer{}
	log.Target = stdlog.New(buf, "JUJU:", 0)
	input := "Hello World"
	log.Infof(input)
	c.Assert(buf.String(), Equals, "JUJU:INFO: "+input+"\n")
	buf.Reset()
	log.Warningf(input)
	c.Assert(buf.String(), Equals, "JUJU:WARNING: "+input+"\n")
	buf.Reset()
	log.Noticef(input)
	c.Assert(buf.String(), Equals, "JUJU:NOTICE: "+input+"\n")
	buf.Reset()
	log.Alertf(input)
	c.Assert(buf.String(), Equals, "JUJU:ALERT: "+input+"\n")
	buf.Reset()
	log.Criticalf(input)
	c.Assert(buf.String(), Equals, "JUJU:CRITICAL: "+input+"\n")
	buf.Reset()
	log.Emergencyf(input)
	c.Assert(buf.String(), Equals, "JUJU:EMERGENCY: "+input+"\n")
	buf.Reset()
	log.Errorf(input)
	c.Assert(buf.String(), Equals, "JUJU:ERROR: "+input+"\n")
	buf.Reset()
}
