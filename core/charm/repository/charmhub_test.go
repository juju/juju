// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/juju/charm/v13"
	charmresource "github.com/juju/charm/v13/resource"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/hash"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/charm/repository/mocks"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

var (
	expRefreshFields = set.NewStrings(
		"download", "id", "license", "name", "publisher", "resources",
		"revision", "summary", "type", "version", "bases", "config-yaml",
		"metadata-yaml",
	).SortedValues()
)

type charmHubRepositorySuite struct {
	testing.IsolationSuite

	client *mocks.MockCharmHubClient
	logger *mocks.MockLogger
}

var _ = gc.Suite(&charmHubRepositorySuite{})

func (s *charmHubRepositorySuite) TestResolveForDeploy(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectCharmRefreshInstallOneFromChannel(c)
	// The origin.ID should never be saved to the origin during
	// ResolveWithPreferredChannel.  That is done during the file
	// download only.
	s.testResolve(c, "")
}

func (s *charmHubRepositorySuite) TestResolveForUpgrade(c *gc.C) {
	defer s.setupMocks(c).Finish()
	cfg, err := charmhub.RefreshOne("instance-key", "charmCHARMcharmCHARMcharmCHARM01", 16, "latest/stable", charmhub.RefreshBase{
		Architecture: arch.DefaultArchitecture,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg)
	// If the origin has an ID, ensure it's kept thru the call
	// to ResolveWithPreferredChannel.
	s.testResolve(c, "charmCHARMcharmCHARMcharmCHARM01")
}

func (s *charmHubRepositorySuite) testResolve(c *gc.C, id string) {
	curl := charm.MustParseURL("ch:wordpress")
	rev := 16
	channel := corecharm.MustParseChannel("stable")
	origin := corecharm.Origin{
		Source:   "charm-hub",
		ID:       id,
		Revision: &rev,
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "20.04",
		},
		Channel: &channel,
	}
	if id != "" {
		origin.InstanceKey = "instance-key"
	}
	if id != "" {
		origin.InstanceKey = "instance-key"
	}

	obtainedCurl, obtainedOrigin, obtainedBases, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = rev

	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Platform.Architecture = arch.DefaultArchitecture
	origin.Platform.OS = "ubuntu"
	origin.Platform.Channel = "20.04"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedBases, jc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveWithChannel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectCharmRefreshInstallOneFromChannel(c)

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

	obtainedCurl, obtainedOrigin, obtainedBases, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, jc.ErrorIsNil)

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

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedBases, jc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveWithoutBase(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectCharmRefreshInstallOneFromChannel(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	obtainedCurl, obtainedOrigin, obtainedBases, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16

	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Platform.Architecture = arch.DefaultArchitecture

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedBases, jc.SameContents, []corecharm.Platform{})
}

func (s *charmHubRepositorySuite) TestResolveForDeployWithRevisionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectCharmRefreshInstallOneByRevisionResources(c)

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

	obtainedData, err := s.newClient().ResolveForDeploy(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = revision

	expectedOrigin := origin
	expectedOrigin.Type = "charm"
	expectedOrigin.Revision = &revision

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedData.URL, jc.DeepEquals, expected)
	c.Assert(obtainedData.EssentialMetadata.ResolvedOrigin, jc.DeepEquals, expectedOrigin)
}

func (s *charmHubRepositorySuite) TestResolveForDeploySuccessChooseBase(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshInvalidPlatformError(c)
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

	obtainedData, err := s.newClient().ResolveForDeploy(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16

	expectedOrigin := origin
	expectedOrigin.Type = "charm"
	expectedOrigin.Revision = &curl.Revision
	expectedOrigin.Platform.OS = "ubuntu"
	expectedOrigin.Platform.Channel = "20.04"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedData.URL, jc.DeepEquals, expected)
	c.Assert(obtainedData.EssentialMetadata.ResolvedOrigin, jc.DeepEquals, expectedOrigin)
	c.Assert(obtainedData.Resources, gc.HasLen, 1)
	foundResource := obtainedData.Resources["wal-e"]
	c.Assert(foundResource.Name, gc.Equals, "wal-e")
	c.Assert(foundResource.Path, gc.Equals, "wal-e.snap")
	c.Assert(foundResource.Revision, gc.Equals, 5)
}
func (s *charmHubRepositorySuite) TestResolveWithBundles(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectBundleRefresh(c)

	curl := charm.MustParseURL("ch:core-kubernetes")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	obtainedCurl, obtainedOrigin, obtainedBases, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "core-kubernetes", origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 17

	origin.Type = "bundle"
	origin.Revision = &curl.Revision
	origin.Channel = &charm.Channel{
		Track: "latest",
		Risk:  "stable",
	}
	origin.Platform.Architecture = arch.DefaultArchitecture

	expected := s.expectedCURL(curl, 17, arch.DefaultArchitecture, "")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedBases, jc.SameContents, []corecharm.Platform{})
}

func (s *charmHubRepositorySuite) TestResolveInvalidPlatformError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshInvalidPlatformError(c)
	s.expectCharmRefreshInstallOneFromChannel(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	obtainedCurl, obtainedOrigin, obtainedBases, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, jc.ErrorIsNil)

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

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedBases, jc.SameContents, []corecharm.Platform{{OS: "ubuntu", Channel: "20.04", Architecture: "amd64"}})
}

func (s *charmHubRepositorySuite) TestResolveRevisionNotFoundErrorWithNoSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError(c)

	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
		},
	}

	_, _, _, err := s.newClient().ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, gc.ErrorMatches,
		`(?m)selecting releases: charm or bundle not found in the charm's default channel, base "amd64"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

func (s *charmHubRepositorySuite) TestResolveRevisionNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError(c)

	origin := corecharm.Origin{
		Source: "charm-hub",
		Platform: corecharm.Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "18.04",
		},
	}

	repo := NewCharmHubRepository(s.logger, s.client)
	_, _, _, err := repo.ResolveWithPreferredChannel(context.Background(), "wordpress", origin)
	c.Assert(err, gc.ErrorMatches,
		`(?m)selecting releases: charm or bundle not found in the charm's default channel, base "amd64/ubuntu/18.04"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

func (s *charmHubRepositorySuite) TestDownloadCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
		Hash:   "SHA256 hash",
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

	resolvedURL, err := url.Parse("ch:amd64/focal/wordpress-42")
	c.Assert(err, jc.ErrorIsNil)
	resolvedArchive := new(charm.CharmArchive)

	s.expectCharmRefreshInstallOneFromChannel(c)
	s.client.EXPECT().DownloadAndRead(gomock.Any(), resolvedURL, "/tmp/foo").Return(resolvedArchive, nil)

	gotArchive, gotOrigin, err := s.newClient().DownloadCharm(context.Background(), "wordpress", requestedOrigin, "/tmp/foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotArchive, gc.Equals, resolvedArchive) // note: we are using gc.Equals to check the pointers here.
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *charmHubRepositorySuite) TestGetDownloadURL(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
		Hash:   "SHA256 hash",
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

	resolvedURL, err := url.Parse("ch:amd64/focal/wordpress-42")
	c.Assert(err, jc.ErrorIsNil)

	s.expectCharmRefreshInstallOneFromChannel(c)

	gotURL, gotOrigin, err := s.newClient().GetDownloadURL(context.Background(), "wordpress", requestedOrigin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL, gc.DeepEquals, resolvedURL)
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *charmHubRepositorySuite) TestGetEssentialMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

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

	s.expectCharmRefreshInstallOneFromChannel(c) // resolve the origin
	s.expectCharmRefreshInstallOneFromChannel(c) // refresh and get metadata

	got, err := s.newClient().GetEssentialMetadata(context.Background(), corecharm.MetadataRequest{
		CharmName: "wordpress",
		Origin:    requestedOrigin,
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 1)
	c.Assert(got[0].Meta.Name, gc.Equals, "wordpress")
	c.Assert(got[0].Config.Options["blog-title"], gc.Not(gc.IsNil))
	c.Assert(got[0].Manifest.Bases, gc.HasLen, 1)
	c.Assert(got[0].ResolvedOrigin.ID, gc.Equals, "", gc.Commentf("ID is only added after charm download"))
	c.Assert(got[0].ResolvedOrigin.Hash, gc.Equals, "", gc.Commentf("Hash is only added after charm download"))
}

func (s *charmHubRepositorySuite) TestResolveResources(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(true)
	s.expectListResourceRevisions(2)

	result, err := s.newClient().ResolveResources(context.Background(), []charmresource.Resource{{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
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

func (s *charmHubRepositorySuite) TestResolveResourcesFromStore(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(false)
	s.expectListResourceRevisions(1)

	id := charmID()
	id.Origin.ID = ""
	result, err := s.newClient().ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}}, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}})
}

func (s *charmHubRepositorySuite) TestResolveResourcesFromStoreNoRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefreshWithRevision(1, true)

	result, err := s.newClient().ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: -1,
		Size:     0,
	}}, charmID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    1,
		Fingerprint: fp(c),
		Size:        0,
	}})
}

func (s *charmHubRepositorySuite) TestResolveResourcesNoMatchingRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(true)
	s.expectRefreshWithRevision(99, true)
	s.expectListResourceRevisions(3)

	_, err := s.newClient().ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginStore,
		Revision: 1,
		Size:     0,
	}}, charmID())
	c.Assert(err, gc.ErrorMatches, `charm resource "wal-e" at revision 1 not found`)
}

func (s *charmHubRepositorySuite) TestResolveResourcesUpload(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(false)

	id := charmID()
	id.Origin.ID = ""
	result, err := s.newClient().ResolveResources(context.Background(), []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginUpload,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}}, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []charmresource.Resource{{
		Meta:     charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:   charmresource.OriginUpload,
		Revision: 3,
		Fingerprint: charmresource.Fingerprint{
			Fingerprint: hash.Fingerprint{}},
		Size: 0,
	}})
}

func (s *charmHubRepositorySuite) TestResourceInfo(c *gc.C) {
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

	result, err := s.newClient().resourceInfo(context.Background(), curl, origin, "wal-e", 25)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, charmresource.Resource{
		Meta:        charmresource.Meta{Name: "wal-e", Type: 1, Path: "wal-e.snap", Description: "WAL-E Snap Package"},
		Origin:      charmresource.OriginStore,
		Revision:    25,
		Fingerprint: fp(c),
		Size:        0,
	})
}

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneFromChannel(c *gc.C) {
	cfg, err := charmhub.InstallOneFromChannel("wordpress", "latest/stable", charmhub.RefreshBase{
		Architecture: arch.DefaultArchitecture,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg)
}

func (s *charmHubRepositorySuite) expectCharmRefresh(c *gc.C, cfg charmhub.RefreshConfig) {
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
					URL:        "ch:amd64/focal/wordpress-42",
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
			},
			EffectiveChannel: "latest/stable",
		}}, nil
	})
}

func (s *charmHubRepositorySuite) expectBundleRefresh(c *gc.C) {
	cfg, err := charmhub.InstallOneFromChannel("core-kubernetes", "latest/stable", charmhub.RefreshBase{
		Architecture: arch.DefaultArchitecture,
	})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *charmHubRepositorySuite) expectedRefreshInvalidPlatformError(c *gc.C) {
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

func (s *charmHubRepositorySuite) expectedRefreshRevisionNotFoundError(c *gc.C) {
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

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneFromChannelFullBase(c *gc.C) {
	cfg, err := charmhub.InstallOneFromChannel("wordpress", "latest/stable", charmhub.RefreshBase{
		Architecture: arch.DefaultArchitecture, Name: "ubuntu", Channel: "20.04",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmRefreshFullWithResources(c, cfg)
}

func (s *charmHubRepositorySuite) expectCharmRefreshInstallOneByRevisionResources(c *gc.C) {
	cfg, err := charmhub.InstallOneFromRevision("wordpress", 16)
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmRefresh(c, cfg)
}

func (s *charmHubRepositorySuite) expectCharmRefreshFullWithResources(c *gc.C, cfg charmhub.RefreshConfig) {
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
					URL:        "ch:amd64/focal/wordpress-42",
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

func (s *charmHubRepositorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockCharmHubClient(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	return ctrl
}

func (s *charmHubRepositorySuite) expectedCURL(curl *charm.URL, revision int, arch string, series string) *charm.URL {
	return curl.WithRevision(revision).WithArchitecture(arch).WithSeries(series)
}

func (s *charmHubRepositorySuite) newClient() *CharmHubRepository {
	return NewCharmHubRepository(s.logger, s.client)
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
	h, err := config.Build()
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

func (s *charmHubRepositorySuite) expectListResourceRevisions(rev int) {
	resp := []transport.ResourceRevision{
		resourceRevision(rev),
	}
	s.client.EXPECT().ListResourceRevisions(gomock.Any(), gomock.Any(), gomock.Any()).Return(resp, nil)
}

func fp(c *gc.C) charmresource.Fingerprint {
	fp, err := charmresource.ParseFingerprint("38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b")
	c.Assert(err, jc.ErrorIsNil)
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

// RefreshConfigMatcher is required so that we do check somethings going into
// the refresh method. As instanceKey is private we don't know what it is until
// it's called.
type RefreshConfigMatcher struct {
	c      *gc.C
	Config charmhub.RefreshConfig
}

func (m RefreshConfigMatcher) Matches(x interface{}) bool {
	rc, ok := x.(charmhub.RefreshConfig)
	if !ok {
		return false
	}

	cb, err := m.Config.Build()
	m.c.Assert(err, jc.ErrorIsNil)

	rcb, err := rc.Build()
	m.c.Assert(err, jc.ErrorIsNil)
	m.c.Assert(len(cb.Actions), gc.Equals, len(rcb.Actions))

	if cb.Actions[0].ID == nil && rcb.Actions[0].ID == nil {
		return true
	}
	return cb.Actions[0].ID != nil && rcb.Actions[0].ID != nil && *cb.Actions[0].ID == *rcb.Actions[0].ID
}

func (RefreshConfigMatcher) String() string {
	return "is refresh config"
}

type refreshConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&refreshConfigSuite{})

func (refreshConfigSuite) TestRefreshByChannel(c *gc.C) {
	name := "wordpress"
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
	}

	cfg, err := refreshConfig(name, origin)
	c.Assert(err, jc.ErrorIsNil)

	ch := channel.String()
	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
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

func (refreshConfigSuite) TestRefreshByChannelVersion(c *gc.C) {
	name := "wordpress"
	platform := corecharm.MustParsePlatform("amd64/ubuntu/20.10/latest")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
	}

	cfg, err := refreshConfig(name, origin)
	c.Assert(err, jc.ErrorIsNil)

	ch := channel.String()
	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &name,
			Channel:     &ch,
			Base: &transport.Base{
				Name:         "ubuntu",
				Channel:      "20.10/latest",
				Architecture: "amd64",
			},
		}},
		Context: []transport.RefreshRequestContext{},
		Fields:  expRefreshFields,
	})
}

func (refreshConfigSuite) TestRefreshByRevision(c *gc.C) {
	revision := 1
	name := "wordpress"
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	origin := corecharm.Origin{
		Platform: platform,
		Revision: &revision,
	}

	cfg, err := refreshConfig(name, origin)
	c.Assert(err, jc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
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

func (refreshConfigSuite) TestRefreshByID(c *gc.C) {
	id := "aaabbbccc"
	revision := 1
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	channel := corecharm.MustParseChannel("stable")
	origin := corecharm.Origin{
		Type:        transport.CharmType.String(),
		ID:          id,
		Platform:    platform,
		Revision:    &revision,
		Channel:     &channel,
		InstanceKey: "instance-key",
	}

	cfg, err := refreshConfig("wordpress", origin)
	c.Assert(err, jc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
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
	testing.IsolationSuite
	logger *mocks.MockLogger
}

var _ = gc.Suite(&selectNextBaseSuite{})

func (*selectNextBaseSuite) TestSelectNextBaseWithNoBases(c *gc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases(nil, corecharm.Origin{})
	c.Assert(err, gc.ErrorMatches, `no bases available`)
}

func (*selectNextBaseSuite) TestSelectNextBaseWithInvalidBases(c *gc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "all",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
		},
	})
	c.Assert(err, gc.ErrorMatches, `bases matching architecture "amd64" not found`)
}

func (*selectNextBaseSuite) TestSelectNextBaseWithInvalidBaseChannel(c *gc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
		},
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (*selectNextBaseSuite) TestSelectNextBaseWithInvalidOS(c *gc.C) {
	repo := new(CharmHubRepository)
	_, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
	}}, corecharm.Origin{
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
		},
	})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (*selectNextBaseSuite) TestSelectNextBaseWithValidBases(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, []corecharm.Platform{{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	}})
}

func (*selectNextBaseSuite) TestSelectNextBaseWithValidBasesWithSeries(c *gc.C) {
	repo := new(CharmHubRepository)
	platform, err := repo.selectNextBases([]transport.Base{{
		Architecture: "amd64",
		Name:         "ubuntu",
		Channel:      "focal",
	}, {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, []corecharm.Platform{{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	}})
}

func (*selectNextBaseSuite) TestSelectNextBaseWithCentosBase(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, []corecharm.Platform{{
		Architecture: "amd64",
		OS:           "centos",
		Channel:      "7",
	}})
}

func (*selectNextBaseSuite) TestSelectNextBasesFromReleasesNoReleasesError(c *gc.C) {
	channel := corecharm.MustParseChannel("stable/foo")
	repo := new(CharmHubRepository)
	err := repo.handleRevisionNotFound([]transport.Release{}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, gc.ErrorMatches, `no releases available`)
}

func (*selectNextBaseSuite) TestSelectNextBasesFromReleasesAmbiguousMatchError(c *gc.C) {
	channel := corecharm.MustParseChannel("stable/foo")
	repo := new(CharmHubRepository)
	err := repo.handleRevisionNotFound([]transport.Release{
		{},
	}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`ambiguous arch and series with channel %q. specify both arch and series along with channel`, channel.String()))
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesSuggestionError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)

	channel := corecharm.MustParseChannel("stable")
	err := repo.handleRevisionNotFound([]transport.Release{{
		Base: transport.Base{
			Name:         "os",
			Channel:      "series",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{
		Channel: &channel,
	})
	c.Assert(err, gc.ErrorMatches, `charm or bundle not found for channel "stable", base ""`)
}

func (s *selectNextBaseSuite) TestSelectNextBasesFromReleasesSuggestion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	err := repo.handleRevisionNotFound([]transport.Release{{
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
	c.Assert(err, gc.ErrorMatches,
		`charm or bundle not found in the charm's default channel, base "arch"
available releases are:
  channel "latest/stable": available bases are: ubuntu@20.04`)
}

func (s *selectNextBaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.logger = mocks.NewMockLogger(ctrl)
	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	return ctrl
}

type composeSuggestionsSuite struct {
	testing.IsolationSuite
	logger *mocks.MockLogger
}

var _ = gc.Suite(&composeSuggestionsSuite{})

func (s *composeSuggestionsSuite) TestNoReleases(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{}, corecharm.Origin{})
	c.Assert(suggestions, gc.DeepEquals, []string(nil))
}

func (s *composeSuggestionsSuite) TestNoMatchingArch(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{{
		Base: transport.Base{
			Name:         "os",
			Channel:      "series",
			Architecture: "arch",
		},
		Channel: "stable",
	}}, corecharm.Origin{})
	c.Assert(suggestions, gc.DeepEquals, []string(nil))
}

func (s *composeSuggestionsSuite) TestSuggestion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{{
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
	c.Assert(suggestions, gc.DeepEquals, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04`,
	})
}

func (s *composeSuggestionsSuite) TestSuggestionWithRisk(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{{
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
	c.Assert(suggestions, gc.DeepEquals, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04`,
	})
}

func (s *composeSuggestionsSuite) TestMultipleSuggestion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{{
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
	c.Assert(suggestions, jc.SameContents, []string{
		`channel "latest/stable": available bases are: ubuntu@20.04, ubuntu@18.04`,
		`channel "2.0/stable": available bases are: ubuntu@18.04`,
	})
}

func (s *composeSuggestionsSuite) TestCentosSuggestion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	repo := NewCharmHubRepository(s.logger, nil)
	suggestions := repo.composeSuggestions([]transport.Release{{
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
	c.Assert(suggestions, gc.DeepEquals, []string{
		`channel "latest/stable": available bases are: centos@7`,
	})
}

func (s *composeSuggestionsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.logger = mocks.NewMockLogger(ctrl)
	s.logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	return ctrl
}
