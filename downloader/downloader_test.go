// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/testing"
)

type suite struct {
	testing.LoggingSuite
	testing.HTTPSuite
}

func (s *suite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *suite) TearDownSuite(c *C) {
	s.HTTPSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *suite) TearDownTest(c *C) {
	s.HTTPSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

var _ = Suite(&suite{})

func Test(t *stdtesting.T) {
	TestingT(t)
}

func (s *suite) TestDownload(c *C) {
	tmp := c.MkDir()
	testing.Server.Response(200, nil, []byte("archive"))
	d := downloader.New(s.URL("/archive.tgz"), tmp)
	status := <-d.Done()
	c.Assert(status.Err, IsNil)
	c.Assert(status.File, NotNil)
	defer os.Remove(status.File.Name())
	defer status.File.Close()

	dir, _ := filepath.Split(status.File.Name())
	c.Assert(filepath.Clean(dir), Equals, tmp)
	assertFileContents(c, status.File, "archive")
}

func (s *suite) TestDownloadError(c *C) {
	testing.Server.Response(404, nil, nil)
	d := downloader.New(s.URL("/archive.tgz"), c.MkDir())
	status := <-d.Done()
	c.Assert(status.File, IsNil)
	c.Assert(status.Err, ErrorMatches, `cannot download ".*": bad http response: 404 Not Found`)
}

func (s *suite) TestStopDownload(c *C) {
	tmp := c.MkDir()
	d := downloader.New(s.URL("/x.tgz"), tmp)
	d.Stop()
	select {
	case status := <-d.Done():
		c.Fatalf("received status %#v after stop", status)
	case <-time.After(testing.ShortWait):
	}
	infos, err := ioutil.ReadDir(tmp)
	c.Assert(err, IsNil)
	c.Assert(infos, HasLen, 0)
}

func assertFileContents(c *C, f *os.File, expect string) {
	got, err := ioutil.ReadAll(f)
	c.Assert(err, IsNil)
	if !c.Check(string(got), Equals, expect) {
		info, err := f.Stat()
		c.Assert(err, IsNil)
		c.Logf("info %#v", info)
	}
}
