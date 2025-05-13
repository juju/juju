// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/hash"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository/mocks"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

var (
	expRefreshFields = set.NewStrings(
		"download", "id", "license", "name", "publisher", "resources",
		"revision", "summary", "type", "version", "bases", "config-yaml",
		"metadata-yaml",
	).SortedValues()
)

type charmHubRepositorySuite struct {
	testhelpers.IsolationSuite

	client *mocks.MockCharmHubClient
}

var _ = tc.Suite(&charmHubRepositorySuite{})

func (s *charmHubRepositorySuite) TestResolveForDeploy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	curl := charm.MustParseURL("ch:wordpress")
	rev := 16

	channel := corecharm.MustParseChannel("latest/stable")
	origin := corecharm.Origin{
		// Notice that there is no ID.
		ID: "",

		Source:   "charm-hub",
		Revision: &rev,
		Hash:     hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	}

	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expectedCURL(curl, rev, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, corecharm.Origin{
		Type:     "charm",
		Source:   "charm-hub",
		Revision: &rev,
		Hash:     hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	})
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
	c.Assert(resolvedData.EssentialMetadata.DownloadInfo, tc.DeepEquals, corecharm.DownloadInfo{
		CharmhubIdentifier: "charmCHARMcharmCHARMcharmCHARM01",
		DownloadURL:        "http://example.com/wordpress-42",
		DownloadSize:       42,
	})
}

func (s *charmHubRepositorySuite) TestResolveForUpgrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	curl := charm.MustParseURL("ch:wordpress")
	rev := 16

	channel := corecharm.MustParseChannel("latest/stable")
	origin := corecharm.Origin{
		// Notice that there is an ID.
		ID: "charmCHARMcharmCHARMcharmCHARM01",

		Source:   "charm-hub",
		Revision: &rev,
		Hash:     hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	}

	cfg, err := charmhub.RefreshOne(context.Background(),
		"instance-key", "charmCHARMcharmCHARMcharmCHARM01", 16, "latest/stable", charmhub.RefreshBase{
			Architecture: arch.DefaultArchitecture,
		})
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg, hash)

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expectedCURL(curl, rev, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, corecharm.Origin{
		Type:     "charm",
		Source:   "charm-hub",
		ID:       "charmCHARMcharmCHARMcharmCHARM01",
		Revision: &rev,
		Hash:     hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	})
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveFillsInEmptyTrack(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	channel := corecharm.MustParseChannel("stable")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	}
	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resolvedData.Origin.Channel.Track, tc.Equals, "latest")
}

func (s *charmHubRepositorySuite) TestResolveWithChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = 16

	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Hash = hash
	origin.Platform.Architecture = arch.DefaultArchitecture
	origin.Platform.OS = "ubuntu"
	origin.Platform.Channel = "20.04"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, origin)
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveWithoutBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = 16

	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Hash = hash
	origin.Platform.Architecture = arch.DefaultArchitecture

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, origin)
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{})
}

func (s *charmHubRepositorySuite) TestResolveForDeployWithRevisionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	s.expectCharmRefreshInstallOneByRevisionResources(c, hash)

	revision := 16
	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Revision: &revision,
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}
	arg := corecharm.CharmID{URL: curl, Origin: origin}

	obtainedData, err := s.newClient(c).ResolveForDeploy(context.Background(), arg)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = revision

	expectedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Revision: &revision,
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		Hash: hash,
	}
	expectedOrigin.Type = "charm"
	expectedOrigin.Revision = &revision

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture)

	c.Check(obtainedData.URL, tc.DeepEquals, expected)
	c.Check(obtainedData.EssentialMetadata.ResolvedOrigin, tc.DeepEquals, expectedOrigin)
	c.Check(obtainedData.EssentialMetadata.DownloadInfo, tc.DeepEquals, corecharm.DownloadInfo{
		CharmhubIdentifier: "charmCHARMcharmCHARMcharmCHARM01",
		DownloadURL:        "http://example.com/wordpress-42",
		DownloadSize:       42,
	})
}

func (s *charmHubRepositorySuite) TestResolveForDeploySuccessChooseBase(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshInvalidPlatformError()
	s.expectCharmRefreshInstallOneFromChannelFullBase(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}
	arg := corecharm.CharmID{URL: curl, Origin: origin}

	obtainedData, err := s.newClient(c).ResolveForDeploy(context.Background(), arg)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = 16

	expectedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		Hash: "SHA256 hash",
	}
	expectedOrigin.Type = "charm"
	expectedOrigin.Revision = &curl.Revision
	expectedOrigin.Platform.OS = "ubuntu"
	expectedOrigin.Platform.Channel = "20.04"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture)

	c.Check(obtainedData.URL, tc.DeepEquals, expected)
	c.Check(obtainedData.EssentialMetadata.ResolvedOrigin, tc.DeepEquals, expectedOrigin)
	c.Check(obtainedData.EssentialMetadata.DownloadInfo, tc.DeepEquals, corecharm.DownloadInfo{
		CharmhubIdentifier: "charmCHARMcharmCHARMcharmCHARM01",
		DownloadURL:        "http://example.com/wordpress-42",
		DownloadSize:       42,
	})

	c.Assert(obtainedData.Resources, tc.HasLen, 1)
	foundResource := obtainedData.Resources["wal-e"]
	c.Check(foundResource.Name, tc.Equals, "wal-e")
	c.Check(foundResource.Path, tc.Equals, "wal-e.snap")
	c.Check(foundResource.Revision, tc.Equals, 5)
}
func (s *charmHubRepositorySuite) TestResolveWithBundles(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectBundleRefresh(c)

	curl := charm.MustParseURL("ch:core-kubernetes")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "core-kubernetes", origin)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = 17

	origin.Type = "bundle"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Platform.Architecture = arch.DefaultArchitecture

	expected := s.expectedCURL(curl, 17, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, origin)
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{})
}

func (s *charmHubRepositorySuite) TestResolveInvalidPlatformError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	s.expectedRefreshInvalidPlatformError()
	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	resolvedData, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	curl.Revision = 16

	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Platform.Architecture = arch.DefaultArchitecture
	origin.Platform.OS = "ubuntu"
	origin.Platform.Channel = "20.04"
	origin.Hash = hash

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture)

	c.Assert(resolvedData.URL, tc.DeepEquals, expected)
	c.Assert(resolvedData.Origin, tc.DeepEquals, origin)
	c.Assert(resolvedData.Platform, tc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveRevisionNotFoundErrorWithNoSeries(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError()

	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	_, err := s.newClient(c).ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorMatches,
		`(?m)selecting releases: charm or bundle not found in the charm's default channel, base "amd64"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

func (s *charmHubRepositorySuite) TestResolveRevisionNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError()

	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "18.04",
		},
	}

	repo := &CharmHubRepository{
		client: s.client,
		logger: loggertesting.WrapCheckLog(c),
	}
	_, err := repo.ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorMatches,
		`(?m)selecting releases: charm or bundle not found in the charm's default channel, base "amd64/ubuntu/18.04"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

func (s *charmHubRepositorySuite) TestDownload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	requestedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Hash:   hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		ID:     "charmCHARMcharmCHARMcharmCHARM01",
		Hash:   hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}

	resolvedURL, err := url.Parse("http://example.com/wordpress-42")
	c.Assert(err, tc.ErrorIsNil)

	s.expectCharmRefreshInstallOneFromChannel(c, hash)
	s.client.EXPECT().Download(gomock.Any(), resolvedURL, "/tmp/foo", gomock.Any()).Return(&charmhub.Digest{
		SHA256: hash,
		SHA384: "sha-384",
		Size:   10,
	}, nil)

	client := s.newClient(c)

	gotOrigin, digest, err := client.Download(context.Background(), "wordpress", requestedOrigin, "/tmp/foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotOrigin, tc.DeepEquals, resolvedOrigin)
	c.Check(digest, tc.DeepEquals, &charmhub.Digest{
		SHA256: hash,
		SHA384: "sha-384",
		Size:   10,
	})
}

func (s *charmHubRepositorySuite) TestGetDownloadURL(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := uuid.MustNewUUID().String()

	requestedOrigin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}
	resolvedOrigin := corecharm.Origin{
		Source: "charm-hub",
		ID:     "charmCHARMcharmCHARMcharmCHARM01",
		Hash:   hash,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	}

	resolvedURL, err := url.Parse("http://example.com/wordpress-42")
	c.Assert(err, tc.ErrorIsNil)

	s.expectCharmRefreshInstallOneFromChannel(c, hash)

	gotURL, gotOrigin, err := s.newClient(c).GetDownloadURL(context.Background(), "wordpress", requestedOrigin)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURL, tc.DeepEquals, resolvedURL)
	c.Assert(gotOrigin, tc.DeepEquals, resolvedOrigin)
}

func (s *charmHubRepositorySuite) TestResolveResources(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(true)
	s.expectListResourceRevisions(2)

	result, err := s.newClient(c).ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}, {
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    2,
		Fingerprint: fp(c),
		Size:        0,
	}}, charmID())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginUpload,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}, {
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    2,
		Fingerprint: fp(c),
		Size:        0,
	}})
}

func (s *charmHubRepositorySuite) TestResolveResourcesFromStore(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(false)
	s.expectListResourceRevisions(1)

	id := charmID()
	id.Origin.ID = ""
	result, err := s.newClient(c).ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}}, id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}})
}

func (s *charmHubRepositorySuite) TestResolveResourcesFromStoreNoRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefreshWithRevision(1, true)

	result, err := s.newClient(c).ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: -1,
		Size:     0,
	}}, charmID())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}})
}

func (s *charmHubRepositorySuite) TestResolveResourcesNoMatchingRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(true)
	s.expectRefreshWithRevision(99, true)
	s.expectListResourceRevisions(3)

	_, err := s.newClient(c).ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}}, charmID())
	c.Assert(err, tc.ErrorMatches, `charm resource "wal-e" at revision 1 not found`)
}

func (s *charmHubRepositorySuite) TestResolveResourcesUpload(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(false)

	id := charmID()
	id.Origin.ID = ""
	result, err := s.newClient(c).ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginUpload,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}}, id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginUpload,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}})
}

func (s *charmHubRepositorySuite) TestResourceInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefreshWithRevision(25, false)

	curl := charm.MustParseURL("ch:amd64/focal/ubuntu-19")
	rev := curl.Revision
	channel := corecharm.MustParseChannel("stable")
	origin := corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Revision: &rev,
		Channel:  &channel,
		Platform: corecharm.Platform{
			OS:           "ubuntu",
			Channel:      "20.04",
			Architecture: "amd64",
		},
	}

	result, err := s.newClient(c).resourceInfo(context.Background(), curl, origin, "wal-e", 25)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, charmresource.Resource{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    25,
		Fingerprint: fp(c),
		Size:        0,
	})
}

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneFromChannel(c *tc.C, hash string) {
	cfg, err := charmhub.InstallOneFromChannel(context.Background(),
		"wordpress", "latest/stable", charmhub.RefreshBase{
			Architecture: arch.DefaultArchitecture,
		})
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg, hash)
}

func (s *charmHubRepositorySuite) expectCharmRefresh(c *tc.C, cfg charmhub.RefreshConfig, hash string) {
	s.client.EXPECT().Refresh(gomock.Any(), RefreshConfigMatcher{c: c, Config: cfg}).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Entity: transport.RefreshEntity{
				Type:     transport.CharmType,
				ID:       "charmCHARMcharmCHARMcharmCHARM01",
				Name:     "wordpress",
				Revision: 16,
				Download: transport.Download{
					HashSHA256: hash,
					HashSHA384: "SHA384 hash",
					Size:       42,
					URL:        "http://example.com/wordpress-42",
				},
				Bases: []transport.Base{
					{
						Name:         "ubuntu",
						Architecture: "amd64",
						Channel:      "20.04",
					},
				},
				MetadataYAML: `
name: wordpress
summary: Blog engine
description: Blog engine
`[1:],
				ConfigYAML: `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
`[1:],
			},
			EffectiveChannel: "latest/stable",
		}}, nil
	})
}

func (s *charmHubRepositorySuite) expectBundleRefresh(c *tc.C) {
	cfg, err := charmhub.InstallOneFromChannel(context.Background(),
		"core-kubernetes", "latest/stable", charmhub.RefreshBase{
			Architecture: arch.DefaultArchitecture,
		})
	c.Assert(err, tc.ErrorIsNil)
	s.client.EXPECT().Refresh(gomock.Any(), RefreshConfigMatcher{c: c, Config: cfg}).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "bundleBUNDLEbundleBUNDLE01",
			InstanceKey: id,
			Entity: transport.RefreshEntity{
				Type:     transport.BundleType,
				ID:       "bundleBUNDLEbundleBUNDLE01",
				Name:     "core-kubernetes",
				Revision: 17,
			},
			EffectiveChannel: "latest/stable",
		}}, nil
	})
}

func (s *charmHubRepositorySuite) expectedRefreshInvalidPlatformError() {
	s.client.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Error: &transport.APIError{
				Code:    transport.ErrorCodeInvalidCharmBase,
				Message: "invalid charm platform",
				Extra: transport.APIErrorExtra{
					DefaultBases: []transport.Base{{
						Architecture: "amd64",
						Name:         "ubuntu",
						Channel:      "20.04",
					}},
				},
			},
		}}, nil
	})
}

func (s *charmHubRepositorySuite) expectedRefreshRevisionNotFoundError() {
	s.client.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Error: &transport.APIError{
				Code:    transport.ErrorCodeRevisionNotFound,
				Message: "revision not found",
				Extra: transport.APIErrorExtra{
					Releases: []transport.Release{{
						Base: transport.Base{
							Architecture: "amd64",
							Name:         "ubuntu",
							Channel:      "20.04",
						},
						Channel: "stable",
					}},
				},
			},
		}}, nil
	})
}

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneFromChannelFullBase(c *tc.C) {
	cfg, err := charmhub.InstallOneFromChannel(context.Background(), "wordpress", "latest/stable", charmhub.RefreshBase{
		Architecture: arch.DefaultArchitecture, Name: "ubuntu", Channel: "20.04",
	})
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmRefreshFullWithResources(c, cfg)
}

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneByRevisionResources(c *tc.C, hash string) {
	cfg, err := charmhub.InstallOneFromRevision(context.Background(), "wordpress", 16)
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg, hash)
}

func (s *charmHubRepositorySuite) expectCharmRefreshFullWithResources(c *tc.C, cfg charmhub.RefreshConfig) {
	s.client.EXPECT().Refresh(gomock.Any(), RefreshConfigMatcher{c: c, Config: cfg}).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)
		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Entity: transport.RefreshEntity{
				Type:     transport.CharmType,
				ID:       "charmCHARMcharmCHARMcharmCHARM01",
				Name:     "wordpress",
				Revision: 16,
				Download: transport.Download{
					HashSHA256: "SHA256 hash",
					HashSHA384: "SHA384 hash",
					Size:       42,
					URL:        "http://example.com/wordpress-42",
				},
				//
				Bases: []transport.Base{
					{
						Name:         "ubuntu",
						Architecture: "amd64",
						Channel:      "20.04",
					},
				},
				MetadataYAML: `
name: wordpress
summary: Blog engine
description: Blog engine
`[1:],
				ConfigYAML: `
options:
  blog-title: {default: My Title, description: A descriptive title used for the blog., type: string}
`[1:],
				Resources: []transport.ResourceRevision{
					resourceRevision(5),
				},
			},
			EffectiveChannel: "latest/stable",
		}}, nil
	})
}

func (s *charmHubRepositorySuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockCharmHubClient(ctrl)
	return ctrl
}

func (s *charmHubRepositorySuite) expectedCURL(curl *charm.URL, revision int, arch string) *charm.URL {
	return curl.WithRevision(revision).WithArchitecture(arch)
}

func (s *charmHubRepositorySuite) newClient(c *tc.C) *CharmHubRepository {
	return &CharmHubRepository{
		client: s.client,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *charmHubRepositorySuite) expectRefresh(id bool) {
	s.expectRefreshWithRevision(0, id)
}

func (s *charmHubRepositorySuite) expectRefreshWithRevision(rev int, id bool) {
	resp := []transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{
				CreatedAt: time.Date(2020, 7, 7, 9, 39, 44, 132000000, time.UTC),
				Download:  transport.Download{HashSHA256: "c97e1efc5367d2fdcfdf29f4a2243b13765cc9cbdfad19627a29ac903c01ae63", Size: 5487460, URL: "https://api.staging.charmhub.io/api/v1/charms/download/jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD_208.charm"},
				ID:        "jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD",
				Name:      "ubuntu",
				Resources: []transport.ResourceRevision{
					resourceRevision(rev),
				},
				Revision: 19,
				Summary:  "PostgreSQL object-relational SQL database (supported version)",
				Version:  "208",
			},
			EffectiveChannel: "latest/stable",
			Error:            (*transport.APIError)(nil),
			Name:             "postgresql",
			Result:           "download",
		},
	}
	s.client.EXPECT().Refresh(gomock.Any(), charmhubConfigMatcher{id: id}).Return(resp, nil)
}

func (s *charmHubRepositorySuite) expectListResourceRevisions(rev int) {
	resp := []transport.ResourceRevision{
		resourceRevision(rev),
	}
	s.client.EXPECT().ListResourceRevisions(gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
}

type refreshConfigSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&refreshConfigSuite{})

func (s *refreshConfigSuite) TestRefreshByChannel(c *tc.C) {
	name := "wordpress"
	// 'mistakenly' include a risk in the platform
	platform := corecharm.MustParsePlatform("amd64/ubuntu/20.04/stable")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
	}

	cfg, err := refreshConfig(context.Background(), name, origin)
	c.Assert(err, tc.ErrorIsNil)

	ch := channel.String()
	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(build, tc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &name,
			Channel:     &ch,
			Base: &transport.Base{
				Name:         "ubuntu",
				Channel:      "20.04",
				Architecture: "amd64",
			},
		}},
		Context: []transport.RefreshRequestContext{},
		Fields:  expRefreshFields,
	})
}

func (s *refreshConfigSuite) TestRefreshByChannelVersion(c *tc.C) {
	name := "wordpress"
	platform := corecharm.MustParsePlatform("amd64/ubuntu/20.10")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
	}

	cfg, err := refreshConfig(context.Background(), name, origin)
	c.Assert(err, tc.ErrorIsNil)

	ch := channel.String()
	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(build, tc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &name,
			Channel:     &ch,
			Base: &transport.Base{
				Name:         "ubuntu",
				Channel:      "20.10",
				Architecture: "amd64",
			},
		}},
		Context: []transport.RefreshRequestContext{},
		Fields:  expRefreshFields,
	})
}

func (s *refreshConfigSuite) TestRefreshByRevision(c *tc.C) {
	revision := 1
	name := "wordpress"
	platform := corecharm.MustParsePlatform("amd64/ubuntu/20.04")
	origin := corecharm.Origin{
		Platform: platform,
		Revision: &revision,
	}

	cfg, err := refreshConfig(context.Background(), name, origin)
	c.Assert(err, tc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(build, tc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &name,
			Revision:    &revision,
		}},
		Context: []transport.RefreshRequestContext{},
		Fields:  expRefreshFields,
	})
}

func (s *refreshConfigSuite) TestRefreshByID(c *tc.C) {
	id := "aaabbbccc"
	revision := 1
	platform := corecharm.MustParsePlatform("amd64/ubuntu/20.04")
	channel := corecharm.MustParseChannel("stable")
	origin := corecharm.Origin{
		Type:        transport.CharmType.String(),
		ID:          id,
		Platform:    platform,
		Revision:    &revision,
		Channel:     &channel,
		InstanceKey: "instance-key",
	}

	cfg, err := refreshConfig(context.Background(), "wordpress", origin)
	c.Assert(err, tc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(build, tc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "refresh",
			InstanceKey: instanceKey,
			ID:          &id,
		}},
		Context: []transport.RefreshRequestContext{{
			InstanceKey: instanceKey,
			ID:          id,
			Revision:    revision,
			Base: transport.Base{
				Name:         "ubuntu",
				Channel:      "20.04",
				Architecture: "amd64",
			},
			TrackingChannel: channel.String(),
		}},
		Fields: expRefreshFields,
	})
}

type selectNextBaseSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&selectNextBaseSuite{})

func (s *selectNextBaseSuite) TestSelectNextBaseWithNoBases(c *tc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases(nil, corecharm.Origin{})
	c.Assert(err, tc.ErrorMatches, `no bases available`)
}

func (s *selectNextBaseSuite) TestSelectNextBaseWithInvalidBases(c *tc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "all",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
		},
	})
	c.Assert(err, tc.ErrorMatches, `bases matching architecture "amd64" not found`)
}

func (s *selectNextBaseSuite) TestSelectNextBaseWithInvalidBaseChannel(c *tc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
		},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *selectNextBaseSuite) TestSelectNextBaseWithInvalidOS(c *tc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
		},
	})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *selectNextBaseSuite) TestSelectNextBaseWithValidBases(c *tc.C) {
	repo := new(CharmHubRepository)
	platform, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
		Name:         "ubuntu",
		Channel:      "20.04",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(platform, tc.DeepEquals, []corecharm.Platform{{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	}})
}

func (s *selectNextBaseSuite) TestSelectNextBaseWithCentosBase(c *tc.C) {
	repo := new(CharmHubRepository)
	platform, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
		Name:         "centos",
		Channel:      "7",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(platform, tc.DeepEquals, []corecharm.Platform{{
		Architecture: "amd64",
		OS:           "centos",
		Channel:      "7",
	}})
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesNoReleasesError(c *tc.C) {
	channel := corecharm.MustParseChannel("stable/foo")
	repo := new(CharmHubRepository)
	err := repo.handleRevisionNotFound(context.Background(), []transport.Release{}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, tc.ErrorMatches, `no releases available`)
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesAmbiguousMatchError(c *tc.C) {
	channel := corecharm.MustParseChannel("stable/foo")
	repo := new(CharmHubRepository)
	err := repo.handleRevisionNotFound(context.Background(), []transport.Release{
		{},
	}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`ambiguous arch and series with channel %q. specify both arch and series along with channel`, channel.String()))
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesSuggestionError(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}

	channel := corecharm.MustParseChannel("stable")
	err := repo.handleRevisionNotFound(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "os",
			Channel:      "series",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, tc.ErrorMatches, `charm or bundle not found for channel "stable", base ""`)
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesSuggestion(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	err := repo.handleRevisionNotFound(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "20.04",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arch",
		},
	})
	c.Assert(err, tc.ErrorMatches,
		`charm or bundle not found in the charm's default channel, base "arch"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

type composeSuggestionsSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&composeSuggestionsSuite{})

func (s *composeSuggestionsSuite) TestNoReleases(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{}, corecharm.Origin{})
	c.Assert(suggestions, tc.DeepEquals, []string(nil))
}

func (s *composeSuggestionsSuite) TestNoMatchingArch(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "os",
			Channel:      "series",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{})
	c.Assert(suggestions, tc.DeepEquals, []string(nil))
}

func (s *composeSuggestionsSuite) TestSuggestion(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "20.04",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arch",
		},
	})
	c.Assert(suggestions, tc.DeepEquals, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04`,
	})
}

func (s *composeSuggestionsSuite) TestSuggestionWithRisk(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "20.04/stable",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "arch",
		},
	})
	c.Assert(suggestions, tc.DeepEquals, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04`,
	})
}

func (s *composeSuggestionsSuite) TestMultipleSuggestion(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "20.04",
			Architecture: "c",
		},
		Channel: "stable",
	}, {
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "18.04",
			Architecture: "c",
		},
		Channel: "stable",
	}, {
		Base: transport.Base{
			Name:         "ubuntu",
			Channel:      "18.04",
			Architecture: "all",
		},
		Channel: "2.0/stable",
	}, {
		Base: transport.Base{
			Name:         "g",
			Channel:      "h",
			Architecture: "i",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "c",
		},
	})
	c.Assert(suggestions, tc.SameContents, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04, ubuntu@18.04`,
		`channel "2.0/stable": available bases are: ubuntu@18.04`,
	})
}

func (s *composeSuggestionsSuite) TestCentosSuggestion(c *tc.C) {
	repo := &CharmHubRepository{
		logger: loggertesting.WrapCheckLog(c),
	}
	suggestions := repo.composeSuggestions(context.Background(), []transport.Release{{
		Base: transport.Base{
			Name:         "centos",
			Channel:      "7",
			Architecture: "c",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "c",
		},
	})
	c.Assert(suggestions, tc.DeepEquals, []string{
		`channel "latest/stable": available bases are: centos@7`,
	})
}

// RefreshConfigMatcher is required so that we do check somethings going into
// the refresh method. As instanceKey is private we don't know what it is until
// it's called.
type RefreshConfigMatcher struct {
	c      *tc.C
	Config charmhub.RefreshConfig
}

func (m RefreshConfigMatcher) Matches(x interface{}) bool {
	rc, ok := x.(charmhub.RefreshConfig)
	if !ok {
		return false
	}

	cb, err := m.Config.Build(context.Background())
	m.c.Assert(err, tc.ErrorIsNil)

	rcb, err := rc.Build(context.Background())
	m.c.Assert(err, tc.ErrorIsNil)
	m.c.Assert(len(cb.Actions), tc.Equals, len(rcb.Actions))

	if cb.Actions[0].ID == nil && rcb.Actions[0].ID == nil {
		return true
	}
	return cb.Actions[0].ID != nil && rcb.Actions[0].ID != nil && *cb.Actions[0].ID == *rcb.Actions[0].ID
}

func (m RefreshConfigMatcher) String() string {
	return m.Config.String()
}

// charmhubConfigMatcher matches only the charm IDs and revisions of a
// charmhub.RefreshMany config.
type charmhubConfigMatcher struct {
	id bool
}

func (m charmhubConfigMatcher) Matches(x interface{}) bool {
	config, ok := x.(charmhub.RefreshConfig)
	if !ok {
		return false
	}
	h, err := config.Build(context.Background())
	if err != nil {
		return false
	}
	if m.id && h.Actions[0].ID != nil && *h.Actions[0].ID == "meshuggah" {
		return true
	}
	if !m.id && h.Actions[0].Name != nil && *h.Actions[0].Name == "ubuntu" {
		return true
	}
	return false
}

func (m charmhubConfigMatcher) String() string {
	if m.id {
		return "match id"
	}
	return "match name"
}

func fp(c *tc.C) charmresource.Fingerprint {
	fp, err := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	c.Assert(err, tc.ErrorIsNil)
	return fp
}

func charmID() corecharm.CharmID {
	curl := charm.MustParseURL("ubuntu")
	channel, _ := charm.ParseChannel("stable")
	return corecharm.CharmID{
		URL: curl,
		Origin: corecharm.Origin{
			ID:      "meshuggah",
			Source:  corecharm.CharmHub,
			Channel: &channel,
			Platform: corecharm.Platform{
				OS:           "ubuntu",
				Channel:      "20.04",
				Architecture: "amd64",
			},
		}}
}

func resourceRevision(rev int) transport.ResourceRevision {
	return transport.ResourceRevision{
		Download: transport.Download{
			HashSHA384: "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
			Size:       0,
			URL:        "https://api.staging.charmhub.io/api/v1/resources/download/charm_jmeJLrjWpJX9OglKSeUHCwgyaCNuoQjD.wal-e_0",
		},
		Name:        "wal-e",
		Revision:    rev,
		Type:        "file",
		Filename:    "wal-e.snap",
		Description: "WAL-E Snap Package",
	}
}
