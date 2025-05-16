// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"
	"net/url"
	"os"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type downloaderSuite struct {
	testhelpers.IsolationSuite

	downloadClient *MockDownloadClient
}

func TestDownloaderSuite(t *stdtesting.T) { tc.Run(t, &downloaderSuite{}) }
func (s *downloaderSuite) TestDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).Return(&charmhub.Digest{
		SHA256: "sha256",
		SHA384: "sha384",
		Size:   123,
	}, nil)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	result, err := downloader.Download(c.Context(), cURL, "sha256")
	c.Assert(err, tc.ErrorIsNil)

	// Ensure the path is not empty and that the temp file still exists.
	c.Assert(result.Path, tc.Not(tc.Equals), "")

	_, err = os.Stat(result.Path)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Size, tc.Equals, int64(123))
}

func (s *downloaderSuite) TestDownloadFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	var tmpPath string

	// Spy on the download call to get the path of the temp file.
	spy := func(_ context.Context, _ *url.URL, path string, _ ...charmhub.DownloadOption) (*charmhub.Digest, error) {
		tmpPath = path
		return &charmhub.Digest{
			SHA256: "sha256-ignored",
			SHA384: "sha384-ignored",
			Size:   123,
		}, errors.Errorf("boom")
	}
	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).DoAndReturn(spy)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	_, err = downloader.Download(c.Context(), cURL, "hash")
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *downloaderSuite) TestDownloadInvalidDigestHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cURL, err := url.Parse("https://example.com/foo")
	c.Assert(err, tc.ErrorIsNil)

	var tmpPath string

	// Spy on the download call to get the path of the temp file.
	spy := func(_ context.Context, _ *url.URL, path string, _ ...charmhub.DownloadOption) (*charmhub.Digest, error) {
		tmpPath = path
		return &charmhub.Digest{
			SHA256: "sha256-ignored",
			SHA384: "sha384-ignored",
			Size:   123,
		}, nil
	}
	s.downloadClient.EXPECT().Download(gomock.Any(), cURL, gomock.Any(), gomock.Any()).DoAndReturn(spy)

	downloader := NewCharmDownloader(s.downloadClient, loggertesting.WrapCheckLog(c))
	_, err = downloader.Download(c.Context(), cURL, "hash")
	c.Assert(err, tc.ErrorIs, ErrInvalidDigestHash)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *downloaderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.downloadClient = NewMockDownloadClient(ctrl)

	return ctrl
}
