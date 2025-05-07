// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	intcharmhub "github.com/juju/juju/internal/charmhub"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/resource/downloader"
)

type CharmHubSuite struct {
	testing.IsolationSuite

	client *MockDownloadClient
}

var _ = tc.Suite(&CharmHubSuite{})

func (s *CharmHubSuite) TestGetResource(c *tc.C) {
	// Arrange:
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = NewMockDownloadClient(ctrl)

	hash := "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b"
	size := int64(42)

	rawURL := "https://api.staging.charmhub.io/api/v1/resource/download/res.name"
	resourceURL, err := url.Parse(rawURL)
	c.Assert(err, jc.ErrorIsNil)

	resourceContent := []byte("resource blob content")

	// Write the resource content to the temporary file when the Download mock
	// is called.
	var path string
	s.client.EXPECT().Download(gomock.Any(), resourceURL, gomock.Any()).Return(
		&intcharmhub.Digest{
			SHA384: hash,
			Size:   size,
		}, nil,
	).Do(
		func(_ context.Context, _ *url.URL, p string, _ ...intcharmhub.DownloadOption) (*intcharmhub.Digest, error) {
			path = p
			// Check that the temporary file has been created.
			_, err = os.Stat(path)
			c.Assert(err, jc.ErrorIsNil)

			// Write the resourceContent to the file, as the Downloader would.
			err := os.WriteFile(path, resourceContent, os.ModeAppend)
			c.Assert(err, jc.ErrorIsNil)

			return nil, nil
		})

	d := downloader.NewResourceDownloader(s.client, loggertesting.WrapCheckLog(c))

	// Act:
	result, err := d.Download(context.Background(), resourceURL, hash, size)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.Bytes(), tc.DeepEquals, resourceContent)

	c.Assert(result.Close(), jc.ErrorIsNil)

	// Check that the file has been deleted on Close.
	_, err = os.Stat(path)
	c.Check(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *CharmHubSuite) TestGetResourceUnexpectedSize(c *tc.C) {
	// Arrange:
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = NewMockDownloadClient(ctrl)

	hash := "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b"
	size := int64(42)

	rawURL := "https://api.staging.charmhub.io/api/v1/resource/download/res.name"
	url, err := url.Parse(rawURL)
	c.Assert(err, jc.ErrorIsNil)

	s.client.EXPECT().Download(gomock.Any(), url, gomock.Any()).Return(
		&intcharmhub.Digest{
			SHA384: hash,
			Size:   -1,
		},
		nil,
	)

	d := downloader.NewResourceDownloader(s.client, loggertesting.WrapCheckLog(c))

	// Act:
	_, err = d.Download(context.Background(), url, hash, size)
	// Assert:
	c.Assert(err, jc.ErrorIs, downloader.ErrUnexpectedSize)
}

func (s *CharmHubSuite) TestGetResourceUnexpectedHash(c *tc.C) {
	// Arrange:
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.client = NewMockDownloadClient(ctrl)

	hash := "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b"
	size := int64(42)

	rawURL := "https://api.staging.charmhub.io/api/v1/resource/download/res.name"
	url, err := url.Parse(rawURL)
	c.Assert(err, jc.ErrorIsNil)

	s.client.EXPECT().Download(gomock.Any(), url, gomock.Any()).Return(
		&intcharmhub.Digest{
			SHA384: "bad-hash",
			Size:   size,
		},
		nil,
	)

	d := downloader.NewResourceDownloader(s.client, loggertesting.WrapCheckLog(c))

	// Act:
	_, err = d.Download(context.Background(), url, hash, size)
	// Assert:
	c.Assert(err, jc.ErrorIs, downloader.ErrUnexpectedHash)
}
