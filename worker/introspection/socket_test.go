// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"regexp"
	"runtime"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/workertest"
)

type suite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&suite{})

func (s *suite) TestConfigValidation(c *gc.C) {
	w, err := introspection.NewWorker(introspection.Config{})
	c.Check(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "empty SocketName not valid")
}

func (s *suite) TestStartStop(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	w, err := introspection.NewWorker(introspection.Config{
		SocketName: "introspection-test",
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, w)
}

type introspectionSuite struct {
	testing.IsolationSuite

	name   string
	worker worker.Worker
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	s.IsolationSuite.SetUpTest(c)

	s.name = "introspection-test"
	w, err := introspection.NewWorker(introspection.Config{
		SocketName: s.name,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.worker = w
	s.AddCleanup(func(c *gc.C) {
		workertest.CheckKill(c, w)
	})
}

func (s *introspectionSuite) call(c *gc.C, url string) []byte {
	path := "@" + s.name
	conn, err := net.Dial("unix", path)
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.0\r\n\r\n", url)
	c.Assert(err, jc.ErrorIsNil)

	buf, err := ioutil.ReadAll(conn)
	c.Assert(err, jc.ErrorIsNil)
	return buf
}

func (s *introspectionSuite) TestCmdLine(c *gc.C) {
	buf := s.call(c, "/debug/pprof/cmdline")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, ".*github.com/juju/juju/worker/introspection/_test/introspection.test")
}

func (s *introspectionSuite) TestGoroutineProfile(c *gc.C) {
	buf := s.call(c, "/debug/pprof/goroutine")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, `^goroutine profile: total \d+`)
}

// matches fails if regex is not found in the contents of b.
// b is expected to be the response from the pprof http server, and will
// contain some HTTP preamble that should be ignored.
func matches(c *gc.C, b []byte, regex string) {
	re, err := regexp.Compile(regex)
	c.Assert(err, jc.ErrorIsNil)
	r := bytes.NewReader(b)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if re.MatchString(sc.Text()) {
			return
		}
	}
	c.Fatalf("%q did not match regex %q", string(b), regex)
}
