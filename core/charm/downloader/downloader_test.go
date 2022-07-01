// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"errors"
	"io/ioutil"
	"net/url"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	corecharm "github.com/juju/juju/v2/core/charm"
)

var _ = gc.Suite(&downloaderSuite{})
var _ = gc.Suite(&downloadedCharmVerificationSuite{})

type downloadedCharmVerificationSuite struct {
	testing.IsolationSuite

	charmArchive *MockCharmArchive
}

func (s *downloadedCharmVerificationSuite) TestVersionMismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("42.0.0"),
	})

	dc := DownloadedCharm{
		Charm: charmArchive,
	}

	err := dc.verify(corecharm.Origin{}, false)
	c.Assert(err, gc.ErrorMatches, ".*min version.*is higher.*")
}

// TestSHA256CheckSkipping ensures that SHA256 checks are skipped when
// downloading charms from charmstore which does not return an expected SHA256
// hash to check against.
func (s *downloadedCharmVerificationSuite) TestSHA256CheckSkipping(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "this-is-not-the-hash-that-you-are-looking-for",
	}

	err := dc.verify(corecharm.Origin{}, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadedCharmVerificationSuite) TestSHA256Mismatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "this-is-not-the-hash-that-you-are-looking-for",
	}

	err := dc.verify(corecharm.Origin{Hash: "the-real-hash"}, false)
	c.Assert(err, gc.ErrorMatches, "detected SHA256 hash mismatch")
}

func (s *downloadedCharmVerificationSuite) TestLXDProfileValidationError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmArchive := NewMockCharmArchive(ctrl)
	charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})

	dc := DownloadedCharm{
		Charm:  charmArchive,
		SHA256: "sha256",
		LXDProfile: &charm.LXDProfile{
			Config: map[string]string{
				"boot": "run-a-keylogger",
			},
		},
	}

	err := dc.verify(corecharm.Origin{Hash: "sha256"}, false)
	c.Assert(err, gc.ErrorMatches, ".*cannot verify charm-provided LXD profile.*")
}

type downloaderSuite struct {
	testing.IsolationSuite
	charmArchive *MockCharmArchive
	repoGetter   *MockRepositoryGetter
	repo         *MockCharmRepository
	storage      *MockStorage
	logger       *MockLogger
}

func (s *downloaderSuite) TestDownloadAndHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tmpFile := filepath.Join(c.MkDir(), "ubuntu-lite.zip")
	c.Assert(ioutil.WriteFile(tmpFile, []byte("meshuggah\n"), 0644), jc.ErrorIsNil)

	curl := charm.MustParseURL("ch:ubuntu-lite")
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}
	resolvedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/candidate")}

	s.repo.EXPECT().DownloadCharm(curl, requestedOrigin, nil, tmpFile).Return(s.charmArchive, resolvedOrigin, nil)
	s.charmArchive.EXPECT().Version().Return("the-version")
	s.charmArchive.EXPECT().LXDProfile().Return(nil)

	dl := s.newDownloader()
	dc, gotOrigin, err := dl.downloadAndHash(curl, requestedOrigin, nil, repoAdapter{s.repo}, tmpFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin, gc.Commentf("expected to get back the resolved origin"))
	c.Assert(dc.SHA256, gc.Equals, "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461")
	c.Assert(dc.Size, gc.Equals, int64(10))
}

func (s downloaderSuite) TestCharmAlreadyStored(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:redis-0")
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}
	knownOrigin := corecharm.Origin{Source: corecharm.CharmHub, ID: "knowncharmhubid", Hash: "knowncharmhash", Channel: mustParseChannel(c, "20.04/candidate")}

	s.storage.EXPECT().PrepareToStoreCharm(curl).Return(
		NewCharmAlreadyStoredError(curl.String()),
	)
	s.repoGetter.EXPECT().GetCharmRepository(corecharm.CharmHub).Return(repoAdapter{s.repo}, nil)
	retURL, _ := url.Parse(curl.String())
	s.repo.EXPECT().GetDownloadURL(curl, requestedOrigin, nil).Return(retURL, knownOrigin, nil)

	dl := s.newDownloader()
	gotOrigin, err := dl.DownloadAndStore(curl, requestedOrigin, nil, false)
	c.Assert(gotOrigin, gc.DeepEquals, knownOrigin, gc.Commentf("expected to get back the known origin for the existing charm"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s downloaderSuite) TestPrepareToStoreCharmError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:redis-0")
	requestedOrigin := corecharm.Origin{Source: corecharm.CharmHub, Channel: mustParseChannel(c, "20.04/edge")}

	s.storage.EXPECT().PrepareToStoreCharm(curl).Return(
		errors.New("something went wrong"),
	)

	dl := s.newDownloader()
	gotOrigin, err := dl.DownloadAndStore(curl, requestedOrigin, nil, false)
	c.Assert(gotOrigin, gc.DeepEquals, corecharm.Origin{}, gc.Commentf("expected a blank origin when encountering errors"))
	c.Assert(err, gc.ErrorMatches, "something went wrong")
}

func (s downloaderSuite) TestNormalizePlatform(c *gc.C) {
	curl := charm.MustParseURL("ch:ubuntu-lite")
	requestedPlatform := corecharm.Platform{
		Series: "focal",
		OS:     "Ubuntu",
	}

	gotPlatform, err := s.newDownloader().normalizePlatform(curl, requestedPlatform)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotPlatform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		Series:       "focal",
		OS:           "ubuntu", // notice lower case
	})
}

func (s downloaderSuite) TestNormalizePlatformError(c *gc.C) {
	curl := charm.MustParseURL("ch:ubuntu-lite")
	requestedPlatform := corecharm.Platform{
		Series: "utopia-planetia",
		OS:     "Ubuntu",
	}

	_, err := s.newDownloader().normalizePlatform(curl, requestedPlatform)
	c.Assert(err, gc.ErrorMatches, ".*unknown OS for series.*")
}

func (s downloaderSuite) TestDownloadAndStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

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

	s.storage.EXPECT().PrepareToStoreCharm(curl).Return(nil)
	s.storage.EXPECT().Store(curl, gomock.AssignableToTypeOf(DownloadedCharm{})).DoAndReturn(
		func(_ *charm.URL, dc DownloadedCharm) error {
			c.Assert(dc.Size, gc.Equals, int64(10))

			contents, err := ioutil.ReadAll(dc.CharmData)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(contents), gc.DeepEquals, "meshuggah\n", gc.Commentf("read charm contents do not match the data written to disk"))
			c.Assert(dc.CharmVersion, gc.Equals, "the-version")
			c.Assert(dc.SHA256, gc.Equals, "4e97ed7423be2ea12939e8fdd592cfb3dcd4d0097d7d193ef998ab6b4db70461")
			c.Assert(dc.Macaroons, gc.DeepEquals, macaroons)

			return nil
		},
	)
	s.repoGetter.EXPECT().GetCharmRepository(corecharm.CharmHub).Return(repoAdapter{s.repo}, nil)
	s.repo.EXPECT().DownloadCharm(curl, requestedOriginWithPlatform, macaroons, gomock.Any()).DoAndReturn(
		func(_ *charm.URL, requestedOrigin corecharm.Origin, _ macaroon.Slice, archivePath string) (CharmArchive, corecharm.Origin, error) {
			c.Assert(ioutil.WriteFile(archivePath, []byte("meshuggah\n"), 0644), jc.ErrorIsNil)
			return s.charmArchive, resolvedOrigin, nil
		},
	)
	s.charmArchive.EXPECT().Meta().Return(&charm.Meta{
		MinJujuVersion: version.MustParse("0.0.42"),
	})
	s.charmArchive.EXPECT().Version().Return("the-version")
	s.charmArchive.EXPECT().LXDProfile().Return(nil)

	dl := s.newDownloader()
	gotOrigin, err := dl.DownloadAndStore(curl, requestedOrigin, macaroons, false)
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin, gc.Commentf("expected to get back the resolved origin"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloaderSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmArchive = NewMockCharmArchive(ctrl)
	s.repo = NewMockCharmRepository(ctrl)
	s.repoGetter = NewMockRepositoryGetter(ctrl)
	s.storage = NewMockStorage(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.logger.EXPECT().Warningf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	return ctrl
}

func (s *downloaderSuite) newDownloader() *Downloader {
	return NewDownloader(s.logger, s.storage, s.repoGetter)
}

func mustParseChannel(c *gc.C, channel string) *charm.Channel {
	ch, err := charm.ParseChannel(channel)
	c.Assert(err, jc.ErrorIsNil)
	return &ch
}

// repoAdapter is an adapter that allows us to use MockCharmRepository whose
// DownloadCharm method returns a CharmArchive instead of the similarly named
// interface in core/charm (which the package-local version embeds).
//
// This allows us to use a package-local mock for CharmArchive while testing.
type repoAdapter struct {
	repo *MockCharmRepository
}

func (r repoAdapter) DownloadCharm(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice, archivePath string) (corecharm.CharmArchive, corecharm.Origin, error) {
	return r.repo.DownloadCharm(charmURL, requestedOrigin, macaroons, archivePath)
}

func (r repoAdapter) ResolveWithPreferredChannel(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice) (*charm.URL, corecharm.Origin, []string, error) {
	return r.repo.ResolveWithPreferredChannel(charmURL, requestedOrigin, macaroons)
}

func (r repoAdapter) GetDownloadURL(charmURL *charm.URL, requestedOrigin corecharm.Origin, macaroons macaroon.Slice) (*url.URL, corecharm.Origin, error) {
	return r.repo.GetDownloadURL(charmURL, requestedOrigin, macaroons)
}
