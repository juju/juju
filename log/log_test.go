package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log"
	stdlog "log"
	"log/syslog"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type suite struct{}

var _ = Suite(suite{})

func (suite) TestLocalLoggerDebug(c *C) {
	buf := &bytes.Buffer{}
	log.Local = stdlog.New(buf, "JUJU:", 0)
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
	log.Local = stdlog.New(buf, "JUJU:", 0)
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
	log.Critf(input)
	c.Assert(buf.String(), Equals, "JUJU:CRITICAL: "+input+"\n")
	buf.Reset()
	log.Emergf(input)
	c.Assert(buf.String(), Equals, "JUJU:EMERGENCY: "+input+"\n")
	buf.Reset()
	log.Errf(input)
	c.Assert(buf.String(), Equals, "JUJU:ERROR: "+input+"\n")
	buf.Reset()
}

func (suite) TestSysLogger(c *C) {
	done := make(chan string)
	serverAddr := log.StartTestSysLogServer(done)

	logger, err := syslog.Dial("udp", serverAddr, syslog.LOG_INFO, "JUJU")
	if err != nil {
		c.Fatalf("syslog.Dial() failed: %s", err)
	}
	log.SysLog = logger
	input := "Hello World"
	log.Infof(input)
	expected := "<6>JUJU: Hello World\n"
	rcvd := <-done
	if rcvd != expected {
		c.Fatalf("s.Info() = '%q', but wanted '%q'", rcvd, expected)
	}
}
