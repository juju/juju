// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"net/url"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/internal/downloader"
	"github.com/juju/juju/internal/testing"
)

type DownloaderSuite struct {
	testing.BaseSuite
	jujutesting.HTTPSuite
}

func (s *DownloaderSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *DownloaderSuite) TearDownSuite(c *tc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *DownloaderSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *DownloaderSuite) TearDownTest(c *tc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = tc.Suite(&DownloaderSuite{})

func (s *DownloaderSuite) URL(c *tc.C, path string) *url.URL {
	urlStr := s.HTTPSuite.URL(path)
	url, err := url.Parse(urlStr)
	c.Assert(err, tc.ErrorIsNil)
	return url
}

func (s *DownloaderSuite) testStart(c *tc.C, hostnameVerification bool) {
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
	dlr := downloader.New(downloader.NewArgs{
		HostnameVerification: hostnameVerification,
	})
	dl := dlr.Start(downloader.Request{
		URL:       s.URL(c, "/archive.tgz"),
		TargetDir: tmp,
	})
	status := <-dl.Done()
	c.Assert(status.Err, tc.IsNil)
	dir, _ := filepath.Split(status.Filename)
	c.Assert(filepath.Clean(dir), tc.Equals, tmp)
	assertFileContents(c, status.Filename, "archive")
}

func (s *DownloaderSuite) TestDownloadWithoutDisablingSSLHostnameVerification(c *tc.C) {
	s.testStart(c, true)
}

func (s *DownloaderSuite) TestDownloadWithDisablingSSLHostnameVerification(c *tc.C) {
	s.testStart(c, false)
}

func (s *DownloaderSuite) TestDownload(c *tc.C) {
	tmp := c.MkDir()
	jujutesting.Server.Response(200, nil, []byte("archive"))
	dlr := downloader.New(downloader.NewArgs{})
	filename, err := dlr.Download(downloader.Request{
		URL:       s.URL(c, "/archive.tgz"),
		TargetDir: tmp,
	})
	c.Assert(err, tc.ErrorIsNil)
	dir, _ := filepath.Split(filename)
	c.Assert(filepath.Clean(dir), tc.Equals, tmp)
	assertFileContents(c, filename, "archive")
}

func (s *DownloaderSuite) TestDownloadHandles409Responses(c *tc.C) {
	tmp := c.MkDir()
	jujutesting.Server.Response(409, nil, []byte("archive"))
	dlr := downloader.New(downloader.NewArgs{})
	_, err := dlr.Download(downloader.Request{
		URL:       s.URL(c, "/archive.tgz"),
		TargetDir: tmp,
	})
	c.Assert(err, tc.ErrorIs, errors.NotYetAvailable)
}
