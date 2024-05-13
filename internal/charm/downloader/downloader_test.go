// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/charm/v13"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/charm/downloader/mocks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ = gc.Suite(&downloaderSuite{})
var _ = gc.Suite(&downloadedCharmVerificationSuite{})

type downloadedCharmVerificationSuite struct {
	testing.IsolationSuite
}

func (s *downloadedCharmVerificationSuite) TestVersionMismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := mocks.NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("42.0.0"),
	})

	dc := downloader.DownloadedCharm{
		Charm: charmArchive,
	}

	err := dc.Verify(corecharm.Origin{}, false)
	c.Assert(err, gc.ErrorMatches, ".*min version.*is higher.*")
}

// TestSHA256CheckSkipping ensures that SHA256 checks are skipped when
// downloading charms from charmstore which does not return an expected SHA256
// hash to check against.
func (s *downloadedCharmVerificationSuite) TestSHA256CheckSkipping(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := mocks.NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := downloader.DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "this-is-not-the-hash-that-you-are-looking-for",
	}

	err := dc.Verify(corecharm.Origin{}, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadedCharmVerificationSuite) TestSHA256Mismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := mocks.NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := downloader.DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "this-is-not-the-hash-that-you-are-looking-for",
	}

	err := dc.Verify(corecharm.Origin{Hash: "the-real-hash"}, false)
	c.Assert(err, gc.ErrorMatches, "detected SHA256 hash mismatch")
}

func (s *downloadedCharmVerificationSuite) TestLXDProfileValidationError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := mocks.NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := downloader.DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "sha256",
		LXDProfile: &charm.LXDProfile{
			Config: map[string]string{
				"boot": "run-a-keylogger",
			},
		},
	}

	err := dc.Verify(corecharm.Origin{Hash: "sha256"}, false)
	c.Assert(err, gc.ErrorMatches, ".*cannot verify charm-provided LXD profile.*")
}

type downloaderSuite struct {
	testing.IsolationSuite
	charmArchive *mocks.MockCharmArchive
	repoGetter   *mocks.MockRepositoryGetter
	repo         *mocks.MockCharmRepository
	storage      *mocks.MockStorage
}

func (s *downloaderSuite) TestDownloadAndHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tmpFile := filepath.Join(c.MkDir(), "ubuntu-lite.zip")
	c.Assert(os.WriteFile(tmpFile, []byte("meshuggah\n"), 0644), jc.ErrorIsNil)

	name := "ch:ubuntu-lite"
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}
	resolvedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/candidate")}

	s.repo.EXPECT().DownloadCharm(gomock.Any(), name, requestedOrigin, tmpFile).Return(s.charmArchive, resolvedOrigin, nil)
	s.charmArchive.EXPECT().Version().Return("the-version")
	s.charmArchive.EXPECT().LXDProfile().Return(nil)

	dl := s.newDownloader(c)
	dc, gotOrigin, err := dl.DownloadAndHash(context.Background(), name, requestedOrigin, repoAdaptor{s.repo}, tmpFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin, gc.Commentf("expected to get back the resolved origin"))
	c.Assert(dc.SHA256, gc.Equals, "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461")
	c.Assert(dc.Size, gc.Equals, int64(10))
}

func (s downloaderSuite) TestCharmAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:redis-0")
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}
	knownOrigin := corecharm.Origin{Source: corecharm.CharmHub, ID: "knowncharmhubid", Hash: "knowncharmhash", Channel: mustParseChannel(c, "20.04/candidate")}

	s.storage.EXPECT().PrepareToStoreCharm(curl.String()).Return(
		downloader.NewCharmAlreadyStoredError(curl.String()),
	)
	s.repoGetter.EXPECT().GetCharmRepository(gomock.Any(), corecharm.CharmHub).Return(repoAdaptor{s.repo}, nil)
	retURL, _ := url.Parse(curl.String())
	s.repo.EXPECT().GetDownloadURL(gomock.Any(), curl.Name, requestedOrigin).Return(retURL, knownOrigin, nil)

	dl := s.newDownloader(c)
	gotOrigin, err := dl.DownloadAndStore(context.Background(), curl, requestedOrigin, false)
	c.Assert(gotOrigin, gc.DeepEquals, knownOrigin, gc.Commentf("expected to get back the known origin for the existing charm"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s downloaderSuite) TestPrepareToStoreCharmError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:redis-0")
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}

	s.storage.EXPECT().PrepareToStoreCharm(curl.String()).Return(
		errors.New("something went wrong"),
	)

	dl := s.newDownloader(c)
	gotOrigin, err := dl.DownloadAndStore(context.Background(), curl, requestedOrigin, false)
	c.Assert(gotOrigin, gc.DeepEquals, corecharm.Origin{}, gc.Commentf("expected a blank origin when encountering errors"))
	c.Assert(err, gc.ErrorMatches, "something went wrong")
}

func (s downloaderSuite) TestNormalizePlatform(c *gc.C) {
	name := "ubuntu-lite"
	requestedPlatform := corecharm.Platform{
		Channel: "20.04",
		OS:      "Ubuntu",
	}

	gotPlatform, err := s.newDownloader(c).NormalizePlatform(name, requestedPlatform)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPlatform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		Channel:      "20.04",
		OS:           "ubuntu", // notice lower case
	})
}

func (s downloaderSuite) TestDownloadAndStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("ch:ubuntu-lite")
	requestedOrigin := corecharm.Origin{
		Source: corecharm.CharmHub,
	}
	requestedOriginWithPlatform := corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Architecture: "amd64",
		},
	}
	resolvedOrigin := corecharm.Origin{
		Source: corecharm.CharmHub,
		Hash:   "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461",
		Platform: corecharm.Platform{
			Architecture: "amd64",
		},
	}

	c.Log(curl.String())
	s.storage.EXPECT().PrepareToStoreCharm(curl.String()).Return(nil)
	s.storage.EXPECT().Store(gomock.Any(), curl.String(), gomock.AssignableToTypeOf(downloader.DownloadedCharm{})).DoAndReturn(
		func(_ context.Context, _ string, dc downloader.DownloadedCharm) error {
			c.Assert(dc.Size, gc.Equals, int64(10))

			contents, err := io.ReadAll(dc.CharmData)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(contents), gc.DeepEquals, "meshuggah\n", gc.Commentf("read charm contents do not match the data written to disk"))
			c.Assert(dc.CharmVersion, gc.Equals, "the-version")
			c.Assert(dc.SHA256, gc.Equals, "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461")

			return nil
		},
	)
	s.repoGetter.EXPECT().GetCharmRepository(gomock.Any(), corecharm.CharmHub).Return(repoAdaptor{s.repo}, nil)
	s.repo.EXPECT().DownloadCharm(gomock.Any(), curl.Name, requestedOriginWithPlatform, gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, requestedOrigin corecharm.Origin, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error) {
			c.Assert(os.WriteFile(archivePath, []byte("meshuggah\n"), 0644), jc.ErrorIsNil)
			return s.charmArchive, resolvedOrigin, nil
		},
	)
	s.charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})
	s.charmArchive.EXPECT().Version().Return("the-version")
	s.charmArchive.EXPECT().LXDProfile().Return(nil)

	dl := s.newDownloader(c)
	gotOrigin, err := dl.DownloadAndStore(context.Background(), curl, requestedOrigin, false)
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin, gc.Commentf("expected to get back the resolved origin"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmArchive = mocks.NewMockCharmArchive(ctrl)
	s.repo = mocks.NewMockCharmRepository(ctrl)
	s.repoGetter = mocks.NewMockRepositoryGetter(ctrl)
	s.storage = mocks.NewMockStorage(ctrl)
	return ctrl
}

func (s *downloaderSuite) newDownloader(c *gc.C) *downloader.Downloader {
	return downloader.NewDownloader(loggertesting.WrapCheckLog(c), s.storage, s.repoGetter)
}

func mustParseChannel(c *gc.C, channel string) *charm.Channel {
	ch, err := charm.ParseChannel(channel)
	c.Assert(err, jc.ErrorIsNil)
	return &ch
}

// repoAdaptor is an adaptor that allows us to use MockCharmRepository whose
// DownloadCharm method returns a CharmArchive instead of the similarly named
// interface in core/charm (which the package-local version embeds).
//
// This allows us to use a package-local mock for CharmArchive while testing.
type repoAdaptor struct {
	repo *mocks.MockCharmRepository
}

func (r repoAdaptor) DownloadCharm(ctx context.Context, charmName string, requestedOrigin corecharm.Origin, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error) {
	return r.repo.DownloadCharm(ctx, charmName, requestedOrigin, archivePath)
}

func (r repoAdaptor) ResolveWithPreferredChannel(ctx context.Context, charmName string, requestedOrigin corecharm.Origin) (*charm.URL, corecharm.Origin, []corecharm.Platform, error) {
	return r.repo.ResolveWithPreferredChannel(ctx, charmName, requestedOrigin)
}

func (r repoAdaptor) GetDownloadURL(ctx context.Context, charmName string, requestedOrigin corecharm.Origin) (*url.URL, corecharm.Origin, error) {
	return r.repo.GetDownloadURL(ctx, charmName, requestedOrigin)
}
