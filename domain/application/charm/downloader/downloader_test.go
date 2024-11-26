// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type downloaderSuite struct {
	testing.IsolationSuite

	repositoryGetter *MockRepositoryGetter
	repository       *MockCharmRepository
}

var _ = gc.Suite(&downloaderSuite{})

func (s *downloaderSuite) TestDownload(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Type:   "charm",
		Hash:   "hash",
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	}

	s.repository.EXPECT().Download(gomock.Any(), "foo", origin, gomock.Any()).Return(origin, &charmhub.Digest{
		DigestType: charmhub.SHA256,
		Hash:       "hash",
		Size:       123,
	}, nil)

	downloader := NewCharmDownloader(s.repositoryGetter, loggertesting.WrapCheckLog(c))
	result, err := downloader.Download(context.Background(), "foo", origin)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the path is not empty and that the temp file still exists.
	c.Assert(result.Path, gc.Not(gc.Equals), "")

	_, err = os.Stat(result.Path)
	c.Check(err, jc.ErrorIsNil)

	c.Check(result.Origin, gc.DeepEquals, origin)
	c.Check(result.Size, gc.Equals, int64(123))
}

func (s *downloaderSuite) TestDownloadFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Type:   "charm",
		Hash:   "hash",
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	}

	var tmpPath string
	s.repository.EXPECT().Download(gomock.Any(), "foo", origin, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ corecharm.Origin, path string) (corecharm.Origin, *charmhub.Digest, error) {
		tmpPath = path
		return origin, &charmhub.Digest{
			DigestType: charmhub.SHA256,
			Hash:       "downloaded-hash",
			Size:       123,
		}, errors.Errorf("boom")
	})

	downloader := NewCharmDownloader(s.repositoryGetter, loggertesting.WrapCheckLog(c))
	_, err := downloader.Download(context.Background(), "foo", origin)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), jc.IsTrue)
}

func (s *downloaderSuite) TestDownloadInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	origin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Type:   "charm",
		Hash:   "input-hash",
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "22.04/stable",
		},
	}

	var tmpPath string
	s.repository.EXPECT().Download(gomock.Any(), "foo", origin, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, _ corecharm.Origin, path string) (corecharm.Origin, *charmhub.Digest, error) {
		tmpPath = path
		return origin, &charmhub.Digest{
			DigestType: charmhub.SHA256,
			Hash:       "downloaded-hash",
			Size:       123,
		}, nil
	})

	downloader := NewCharmDownloader(s.repositoryGetter, loggertesting.WrapCheckLog(c))
	_, err := downloader.Download(context.Background(), "foo", origin)
	c.Assert(err, jc.ErrorIs, ErrInvalidHash)

	_, err = os.Stat(tmpPath)
	c.Check(os.IsNotExist(err), jc.IsTrue)
}

func (s *downloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.repositoryGetter = NewMockRepositoryGetter(ctrl)
	s.repository = NewMockCharmRepository(ctrl)

	s.repositoryGetter.EXPECT().GetCharmRepository(gomock.Any(), corecharm.CharmHub).Return(s.repository, nil)

	return ctrl
}
