package log_test

import (
	"bytes"
	. "launchpad.net/gocheck"
    "io/ioutil"
	"launchpad.net/juju/go/log"
    "os"
    "path"
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

func patchStderr() (*bytes.Buffer, func()) {
    stderr := *log.StderrPtr
    buf := new(bytes.Buffer)
    *log.StderrPtr = buf
    return buf, func() {
        *log.StderrPtr = stderr
    }
}

var logfileTests = []struct {
	expect string
	debug bool
}{
	{
		expect: "JUJU Salut le monde\nJUJU:DEBUG Au revoir l'espace\n",
		debug: true,
	},
	{
		expect: "JUJU Salut le monde\n",
		debug: false,
	},
}

func (suite) TestSetFile(c *C) {
    buf, cleanup := patchStderr()
    defer cleanup()
	for _, t := range logfileTests {
        buf.Reset()
        logdir, _ := ioutil.TempDir("", "")
        logfile := path.Join(logdir, "log")
        defer os.RemoveAll(logdir)
        ioutil.WriteFile(logfile, []byte("previous\n"), 0644)

        log.Debug = t.debug
        log.SetFile(logfile)
        log.Printf("Salut le monde")
        log.Debugf("Au revoir l'espace")
        c.Assert(buf.String(), Equals, t.expect)
        content, _ := ioutil.ReadFile(logfile)
        c.Assert(string(content), Equals, "previous\n" + t.expect)
    }
}
