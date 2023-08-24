// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/downloader"
	"github.com/juju/juju/testing"
)

type DownloadSuite struct {
	testing.BaseSuite
	jujutesting.HTTPSuite
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

func (s *DownloadSuite) testDownload(c *gc.C, hostnameVerification bool) {
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(hostnameVerification),
	)
	status := <-d.Done()
	c.Assert(status.Err, gc.IsNil)

	dir, _ := filepath.Split(status.Filename)
	c.Assert(filepath.Clean(dir), gc.Equals, tmp)
	assertFileContents(c, status.Filename, "archive")
}

func (s *DownloadSuite) TestDownloadWithoutDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, true)
}

func (s *DownloadSuite) TestDownloadWithDisablingSSLHostnameVerification(c *gc.C) {
	s.testDownload(c, false)
}

func (s *DownloadSuite) TestDownloadError(c *gc.C) {
	jujutesting.Server.Response(404, nil, nil)
	tmp := c.MkDir()
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(true),
	)
	filename, err := d.Wait()
	c.Assert(filename, gc.Equals, "")
	c.Assert(err, gc.ErrorMatches, `bad http response: 404 Not Found`)
	checkDirEmpty(c, tmp)
}

func (s *DownloadSuite) TestVerifyValid(c *gc.C) {
	stub := &jujutesting.Stub{}
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
	dl := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
			Verify: func(f *os.File) error {
				stub.AddCall("Verify", f)
				return nil
			},
		},
		downloader.NewHTTPBlobOpener(true),
	)
	filename, err := dl.Wait()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(filename, gc.Not(gc.Equals), "")
	stub.CheckCallNames(c, "Verify")
}

func (s *DownloadSuite) TestVerifyInvalid(c *gc.C) {
	stub := &jujutesting.Stub{}
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
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
		downloader.NewHTTPBlobOpener(true),
	)
	filename, err := dl.Wait()
	c.Check(filename, gc.Equals, "")
	c.Check(errors.Cause(err), gc.Equals, invalid)
	stub.CheckCallNames(c, "Verify")
	checkDirEmpty(c, tmp)
}

func (s *DownloadSuite) TestAbort(c *gc.C) {
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
	abort := make(chan struct{})
	close(abort)
	dl := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
			Abort:     abort,
		},
		downloader.NewHTTPBlobOpener(true),
	)
	filename, err := dl.Wait()
	c.Check(filename, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "download aborted")
	checkDirEmpty(c, tmp)
}

func assertFileContents(c *gc.C, filename, expect string) {
	got, err := os.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(got), gc.Equals, expect)
}

func checkDirEmpty(c *gc.C, dir string) {
	files, err := os.ReadDir(dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(files, gc.HasLen, 0)
}
