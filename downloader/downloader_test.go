// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"net/url"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/downloader"
	"github.com/juju/juju/testing"
)

type DownloaderSuite struct {
	testing.BaseSuite
	gitjujutesting.HTTPSuite
}

func (s *DownloaderSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.HTTPSuite.SetUpSuite(c)
}

func (s *DownloaderSuite) TearDownSuite(c *gc.C) {
	s.HTTPSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *DownloaderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.HTTPSuite.SetUpTest(c)
}

func (s *DownloaderSuite) TearDownTest(c *gc.C) {
	s.HTTPSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&DownloaderSuite{})

func (s *DownloaderSuite) URL(c *gc.C, path string) *url.URL {
	urlStr := s.HTTPSuite.URL(path)
	URL, err := url.Parse(urlStr)
	c.Assert(err, jc.ErrorIsNil)
	return URL
}

func (s *DownloaderSuite) testStart(c *gc.C, hostnameVerification utils.SSLHostnameVerification) {
	tmp := c.MkDir()
	gitjujutesting.Server.Response(200, nil, []byte("archive"))
	dlr := downloader.New(downloader.NewArgs{
		HostnameVerification: hostnameVerification,
	})
	dl := dlr.Start(downloader.Request{
		URL:       s.URL(c, "/archive.tgz"),
		TargetDir: tmp,
	})
	status := <-dl.Done()
	c.Assert(status.Err, gc.IsNil)
	dir, _ := filepath.Split(status.Filename)
	c.Assert(filepath.Clean(dir), gc.Equals, tmp)
	assertFileContents(c, status.Filename, "archive")
}

func (s *DownloaderSuite) TestDownloadWithoutDisablingSSLHostnameVerification(c *gc.C) {
	s.testStart(c, utils.VerifySSLHostnames)
}

func (s *DownloaderSuite) TestDownloadWithDisablingSSLHostnameVerification(c *gc.C) {
	s.testStart(c, utils.NoVerifySSLHostnames)
}

func (s *DownloaderSuite) TestDownload(c *gc.C) {
	tmp := c.MkDir()
	gitjujutesting.Server.Response(200, nil, []byte("archive"))
	dlr := downloader.New(downloader.NewArgs{})
	filename, err := dlr.Download(downloader.Request{
		URL:       s.URL(c, "/archive.tgz"),
		TargetDir: tmp,
	})
	c.Assert(err, jc.ErrorIsNil)
	dir, _ := filepath.Split(filename)
	c.Assert(filepath.Clean(dir), gc.Equals, tmp)
	assertFileContents(c, filename, "archive")
}
