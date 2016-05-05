// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pprof

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type suite struct {
}

var _ = gc.Suite(&suite{})

func TestSuite(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("skipping pprof tests, %q not supported", runtime.GOOS)
	}
	gc.TestingT(t)
}

func (s *suite) TestFilename(c *gc.C) {
	got := Filename
	want := fmt.Sprintf("pprof.pprof.test.%d", os.Getpid())
	c.Assert(got, gc.Equals, want)
}

func (s *suite) TestPprofStartReturnsNonNilShutdownFn(c *gc.C) {
	stop := Start(filepath.Join(c.MkDir(), Filename))
	c.Assert(stop, gc.NotNil)
	defer stop()
}

func (s *suite) TestPprofStart(c *gc.C) {
	path := filepath.Join(c.MkDir(), Filename)
	_, err := os.Stat(path)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	stop := Start(path)
	_, err = os.Stat(path)
	c.Assert(err, gc.IsNil)

	err = stop()
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(path)
	c.Assert(os.IsNotExist(err), jc.IsTrue)
}

func (s *suite) TestPprofStartWithExistingSocketFile(c *gc.C) {
	path := filepath.Join(c.MkDir(), Filename)
	w, err := os.Create(path)
	c.Assert(err, gc.IsNil)

	w.Write([]byte("not a socket"))
	err = w.Close() // can ignore error from w.Write
	c.Assert(err, gc.IsNil)

	stop := Start(path)
	defer stop()
	fi, err := os.Stat(path)
	c.Assert(err, gc.IsNil)
	c.Assert(fi.Mode()&os.ModeSocket != 0, jc.IsTrue)
}

type pprofSuite struct {
	stop func() error
	path string
}

var _ = gc.Suite(&pprofSuite{})

func (s *pprofSuite) SetUpSuite(c *gc.C) {
	s.path = filepath.Join(c.MkDir(), Filename)
	s.stop = Start(s.path)
}

func (s *pprofSuite) TearDownSuite(c *gc.C) {
	s.stop()
}

func (s *pprofSuite) call(c *gc.C, url string) []byte {
	conn, err := net.Dial("unix", s.path)
	c.Assert(err, gc.IsNil)
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.0\r\n\r\n", url)
	c.Assert(err, gc.IsNil)

	buf, err := ioutil.ReadAll(conn)
	c.Assert(err, gc.IsNil)
	return buf
}

func (s *pprofSuite) TestCmdLine(c *gc.C) {
	buf := s.call(c, "/debug/pprof/cmdline")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, ".*github.com/juju/juju/cmd/pprof/_test/pprof.test")
}

func (s *pprofSuite) TestGoroutineProfile(c *gc.C) {
	buf := s.call(c, "/debug/pprof/goroutine")
	c.Assert(buf, gc.NotNil)
	matches(c, buf, `^goroutine profile: total \d+`)
}

// matches fails if regex is not found in the contents of b.
// b is expected to be the response from the pprof http server, and will
// contain some HTTP preamble that should be ignored.
func matches(c *gc.C, b []byte, regex string) {
	re, err := regexp.Compile(regex)
	c.Assert(err, gc.IsNil)
	r := bytes.NewReader(b)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if re.MatchString(sc.Text()) {
			return
		}
	}
	c.Fatalf("%q did not match regex %q", string(b), regex)
}
