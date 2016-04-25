// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/downloader"
	"github.com/juju/juju/testing"
)

type DownloadSuite struct {
	testing.BaseSuite
	gitjujutesting.HTTPSuite
}

func (s *DownloadSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *DownloadSuite) TearDownSuite(c *gc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *DownloadSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *DownloadSuite) TearDownTest(c *gc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&DownloadSuite{})

func (s *DownloadSuite) URL(c *gc.C, path string) *url.URL {
	urlStr := s.HTTPSuite.URL(path)
	URL, err := url.Parse(urlStr)
	c.Assert(err, jc.ErrorIsNil)
	return URL
}

func (s *DownloadSuite) testDownload(c *gc.C, hostnameVerification utils.SSLHostnameVerification) {
	tmp := c.MkDir()
	gitjujutesting.Server.Response(200, nil, []byte("archive"))
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(hostnameVerification),
	)
	status := <-d.Done()
	defer status.File.Close()
	c.Assert(status.Err, gc.IsNil)
	c.Assert(status.File, gc.NotNil)

	dir, _ := filepath.Split(status.File.Name())
	c.Assert(filepath.Clean(dir), gc.Equals, tmp)
	assertFileContents(c, status.File, "archive")
}

func (s *DownloadSuite) TestDownloadWithoutDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, utils.VerifySSLHostnames)
}

func (s *DownloadSuite) TestDownloadWithDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, utils.NoVerifySSLHostnames)
}

func (s *DownloadSuite) TestDownloadError(c *gc.C) {
	gitjujutesting.Server.Response(404, nil, nil)
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: c.MkDir(),
		},
		downloader.NewHTTPBlobOpener(utils.VerifySSLHostnames),
	)
	status := <-d.Done()
	c.Assert(status.File, gc.IsNil)
	c.Assert(status.Err, gc.ErrorMatches, `cannot download ".*": bad http response: 404 Not Found`)
}

func (s *DownloadSuite) TestStop(c *gc.C) {
	tmp := c.MkDir()
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/x.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(utils.VerifySSLHostnames),
	)
	d.Stop()
	select {
	case status := <-d.Done():
		c.Fatalf("received status %#v after stop", status)
	case <-time.After(testing.ShortWait):
	}
	infos, err := ioutil.ReadDir(tmp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(infos, gc.HasLen, 0)
}

func (s *DownloadSuite) TestVerifyValid(c *gc.C) {
	stub := &gitjujutesting.Stub{}
	tmp := c.MkDir()
	gitjujutesting.Server.Response(200, nil, []byte("archive"))
	dl := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
			Verify: func(f *os.File) error {
				stub.AddCall("Verify", f)
				return nil
			},
		},
		downloader.NewHTTPBlobOpener(utils.VerifySSLHostnames),
	)
	status := <-dl.Done()
	c.Assert(status.Err, jc.ErrorIsNil)

	stub.CheckCallNames(c, "Verify")
	stub.CheckCall(c, 0, "Verify", status.File)
}

func (s *DownloadSuite) TestVerifyInvalid(c *gc.C) {
	stub := &gitjujutesting.Stub{}
	tmp := c.MkDir()
	gitjujutesting.Server.Response(200, nil, []byte("archive"))
	invalid := errors.NotValidf("oops")
	dl := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
			Verify: func(f *os.File) error {
				stub.AddCall("Verify", f)
				return invalid
			},
		},
		downloader.NewHTTPBlobOpener(utils.VerifySSLHostnames),
	)
	status := <-dl.Done()

	c.Check(errors.Cause(status.Err), gc.Equals, invalid)
	stub.CheckCallNames(c, "Verify")
	stub.CheckCall(c, 0, "Verify", status.File)
}

func assertFileContents(c *gc.C, f *os.File, expect string) {
	got, err := ioutil.ReadAll(f)
	c.Assert(err, jc.ErrorIsNil)
	if !c.Check(string(got), gc.Equals, expect) {
		info, err := f.Stat()
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("info %#v", info)
	}
}
