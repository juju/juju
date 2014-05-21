// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type suite struct {
	testing.BaseSuite
	testing.HTTPSuite
}

func (s *suite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *suite) TearDownSuite(c *gc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *suite) TearDownTest(c *gc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&suite{})

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

func (s *suite) testDownload(c *gc.C, hostnameVerification utils.SSLHostnameVerification) {
	tmp := c.MkDir()
	testing.Server.Response(200, nil, []byte("archive"))
	d := downloader.New(s.URL("/archive.tgz"), tmp, hostnameVerification)
	status := <-d.Done()
	c.Assert(status.Err, gc.IsNil)
	c.Assert(status.File, gc.NotNil)
	defer os.Remove(status.File.Name())
	defer status.File.Close()

	dir, _ := filepath.Split(status.File.Name())
	c.Assert(filepath.Clean(dir), gc.Equals, tmp)
	assertFileContents(c, status.File, "archive")
}

func (s *suite) TestDownloadWithoutDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, utils.VerifySSLHostnames)
}

func (s *suite) TestDownloadWithDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, utils.NoVerifySSLHostnames)
}

func (s *suite) TestDownloadError(c *gc.C) {
	testing.Server.Response(404, nil, nil)
	d := downloader.New(s.URL("/archive.tgz"), c.MkDir(), utils.VerifySSLHostnames)
	status := <-d.Done()
	c.Assert(status.File, gc.IsNil)
	c.Assert(status.Err, gc.ErrorMatches, `cannot download ".*": bad http response: 404 Not Found`)
}

func (s *suite) TestStopDownload(c *gc.C) {
	tmp := c.MkDir()
	d := downloader.New(s.URL("/x.tgz"), tmp, utils.VerifySSLHostnames)
	d.Stop()
	select {
	case status := <-d.Done():
		c.Fatalf("received status %#v after stop", status)
	case <-time.After(testing.ShortWait):
	}
	infos, err := ioutil.ReadDir(tmp)
	c.Assert(err, gc.IsNil)
	c.Assert(infos, gc.HasLen, 0)
}

func assertFileContents(c *gc.C, f *os.File, expect string) {
	got, err := ioutil.ReadAll(f)
	c.Assert(err, gc.IsNil)
	if !c.Check(string(got), gc.Equals, expect) {
		info, err := f.Stat()
		c.Assert(err, gc.IsNil)
		c.Logf("info %#v", info)
	}
}
