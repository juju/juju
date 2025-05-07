// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/downloader"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type DownloadSuite struct {
	testing.BaseSuite
	testhelpers.HTTPSuite
}

func (s *DownloadSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *DownloadSuite) TearDownSuite(c *tc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *DownloadSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *DownloadSuite) TearDownTest(c *tc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = tc.Suite(&DownloadSuite{})

func (s *DownloadSuite) URL(c *tc.C, path string) *url.URL {
	urlStr := s.HTTPSuite.URL(path)
	url, err := url.Parse(urlStr)
	c.Assert(err, tc.ErrorIsNil)
	return url
}

func (s *DownloadSuite) testDownload(c *tc.C, hostnameVerification bool) {
	tmp := c.MkDir()
	testhelpers.Server.Response(200, nil, []byte("archive"))
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(hostnameVerification),
	)
	status := <-d.Done()
	c.Assert(status.Err, tc.IsNil)

	dir, _ := filepath.Split(status.Filename)
	c.Assert(filepath.Clean(dir), tc.Equals, tmp)
	assertFileContents(c, status.Filename, "archive")
}

func (s *DownloadSuite) TestDownloadWithoutDisablingSSLHostnameVerification(c *tc.C) {
	s.testDownload(c, true)
}

func (s *DownloadSuite) TestDownloadWithDisablingSSLHostnameVerification(c *tc.C) {
	s.testDownload(c, false)
}

func (s *DownloadSuite) TestDownloadError(c *tc.C) {
	testhelpers.Server.Response(404, nil, nil)
	tmp := c.MkDir()
	d := downloader.StartDownload(
		downloader.Request{
			URL:       s.URL(c, "/archive.tgz"),
			TargetDir: tmp,
		},
		downloader.NewHTTPBlobOpener(true),
	)
	filename, err := d.Wait()
	c.Assert(filename, tc.Equals, "")
	c.Assert(err, tc.ErrorMatches, `bad http response: 404 Not Found`)
	checkDirEmpty(c, tmp)
}

func (s *DownloadSuite) TestVerifyValid(c *tc.C) {
	stub := &testhelpers.Stub{}
	tmp := c.MkDir()
	testhelpers.Server.Response(200, nil, []byte("archive"))
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
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filename, tc.Not(tc.Equals), "")
	stub.CheckCallNames(c, "Verify")
}

func (s *DownloadSuite) TestVerifyInvalid(c *tc.C) {
	stub := &testhelpers.Stub{}
	tmp := c.MkDir()
	testhelpers.Server.Response(200, nil, []byte("archive"))
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
	c.Check(filename, tc.Equals, "")
	c.Check(errors.Cause(err), tc.Equals, invalid)
	stub.CheckCallNames(c, "Verify")
	checkDirEmpty(c, tmp)
}

func (s *DownloadSuite) TestAbort(c *tc.C) {
	tmp := c.MkDir()
	testhelpers.Server.Response(200, nil, []byte("archive"))
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
	c.Check(filename, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "download aborted")
	checkDirEmpty(c, tmp)
}

func assertFileContents(c *tc.C, filename, expect string) {
	got, err := os.ReadFile(filename)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(got), tc.Equals, expect)
}

func checkDirEmpty(c *tc.C, dir string) {
	files, err := os.ReadDir(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(files, tc.HasLen, 0)
}
