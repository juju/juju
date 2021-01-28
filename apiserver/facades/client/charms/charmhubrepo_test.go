// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type charmHubRepositoriesSuite struct {
	testing.IsolationSuite
	client *mocks.MockCharmHubClient
}

var _ = gc.Suite(&charmHubRepositoriesSuite{})

func (s *charmHubRepositoriesSuite) TestResolveWithoutSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectRefresh(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub", Architecture: arch.DefaultArchitecture}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16

	origin.ID = "charmCHARMcharmCHARMcharmCHARM01"
	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Risk = "stable"
	origin.Architecture = arch.DefaultArchitecture

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{""})
}

func (s *charmHubRepositoriesSuite) TestResolveInvalidPlatformError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshInvalidPlatformError(c)
	s.expectRefresh(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub", Architecture: arch.DefaultArchitecture}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16

	origin.ID = "charmCHARMcharmCHARMcharmCHARM01"
	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Risk = "stable"
	origin.Architecture = arch.DefaultArchitecture
	origin.OS = "ubuntu"
	origin.Series = "focal"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"focal"})
}

func (s *charmHubRepositoriesSuite) TestResolveRevisionNotFoundErrorWithNoSeries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub", Architecture: arch.DefaultArchitecture}

	resolver := &chRepo{client: s.client}
	_, _, _, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, gc.ErrorMatches, `refresh: no charm or bundle matching channel or platform; suggestions: stable with amd64/ubuntu/focal`)
}

func (s *charmHubRepositoriesSuite) TestResolveRevisionNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectedRefreshRevisionNotFoundError(c)
	s.expectRefresh(c)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub", Architecture: arch.DefaultArchitecture, Series: "bionic"}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16

	origin.ID = "charmCHARMcharmCHARMcharmCHARM01"
	origin.Type = "charm"
	origin.Revision = &curl.Revision
	origin.Risk = "stable"
	origin.Architecture = arch.DefaultArchitecture
	origin.OS = "ubuntu"
	origin.Series = "focal"

	expected := s.expectedCURL(curl, 16, arch.DefaultArchitecture, "focal")

	c.Assert(obtainedCurl, jc.DeepEquals, expected)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"focal"})
}

func (s *charmHubRepositoriesSuite) expectRefresh(c *gc.C) {
	cfg, err := charmhub.InstallOneFromChannel("wordpress", "latest/stable", charmhub.RefreshPlatform{
		Architecture: arch.DefaultArchitecture,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.client.EXPECT().Refresh(gomock.Any(), RefreshConfgMatcher{c: c, Config: cfg}).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Entity: transport.RefreshEntity{
				ID:       "charmCHARMcharmCHARMcharmCHARM01",
				Name:     "wordpress",
				Revision: 16,
			},
			EffectiveChannel: "latest/stable",
		}}, nil
	})
}

func (s *charmHubRepositoriesSuite) expectedRefreshInvalidPlatformError(c *gc.C) {
	s.client.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		id := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			ID:          "charmCHARMcharmCHARMcharmCHARM01",
			InstanceKey: id,
			Error: &transport.APIError{
				Code:    transport.ErrorCodeInvalidCharmPlatform,
				Message: "invalid charm platform",
				Extra: transport.APIErrorExtra{
					DefaultPlatforms: []transport.Platform{{
						Architecture: "amd64",
						OS:           "ubuntu",
						Series:       "focal",
					}},
				},
			},
		}}, nil
	})
}

func (s *charmHubRepositoriesSuite) expectedRefreshRevisionNotFoundError(c *gc.C) {
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
						Platform: "ubuntu/focal/amd64",
						Channel:  "stable",
					}},
				},
			},
		}}, nil
	})
}

func (s *charmHubRepositoriesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockCharmHubClient(ctrl)
	return ctrl
}

func (s *charmHubRepositoriesSuite) expectedCURL(curl *charm.URL, revision int, arch string, series string) *charm.URL {
	return curl.WithRevision(revision).WithArchitecture(arch).WithSeries(series)
}

// RefreshConfgMatcher is required so that we do check somethings going into
// the refresh method. As instanceKey is private we don't know what it is until
// it's called.
type RefreshConfgMatcher struct {
	c      *gc.C
	Config charmhub.RefreshConfig
}

func (m RefreshConfgMatcher) Matches(x interface{}) bool {
	rc, ok := x.(charmhub.RefreshConfig)
	if !ok {
		return false
	}

	cb, _, err := m.Config.Build()
	m.c.Assert(err, jc.ErrorIsNil)

	rcb, _, err := rc.Build()
	m.c.Assert(err, jc.ErrorIsNil)
	m.c.Assert(len(cb.Actions), gc.Equals, len(rcb.Actions))

	return cb.Actions[0].ID == rcb.Actions[0].ID
}

func (RefreshConfgMatcher) String() string {
	return "is refresh config"
}

type refreshConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&refreshConfigSuite{})

func (refreshConfigSuite) TestRefreshByChannel(c *gc.C) {
	curl := charm.MustParseURL("ch:wordpress")
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
	}

	cfg, err := refreshConfig(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	ch := channel.String()
	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, _, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &curl.Name,
			Channel:     &ch,
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "amd64",
			},
		}},
		Context: []transport.RefreshRequestContext{},
	})
}

func (refreshConfigSuite) TestRefreshByRevision(c *gc.C) {
	revision := 1
	curl := charm.MustParseURL("ch:wordpress")
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		Platform: platform,
		Channel:  &channel,
		Revision: &revision,
	}

	cfg, err := refreshConfig(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, _, err := cfg.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(build, gc.DeepEquals, transport.RefreshRequest{
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: instanceKey,
			Name:        &curl.Name,
			Revision:    &revision,
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "amd64",
			},
		}},
		Context: []transport.RefreshRequestContext{},
	})
}

func (refreshConfigSuite) TestRefreshByID(c *gc.C) {
	id := "aaabbbccc"
	revision := 1
	curl := charm.MustParseURL("ch:wordpress")
	platform := corecharm.MustParsePlatform("amd64/ubuntu/focal")
	channel := corecharm.MustParseChannel("latest/stable").Normalize()
	origin := corecharm.Origin{
		ID:       id,
		Platform: platform,
		Channel:  &channel,
		Revision: &revision,
	}

	cfg, err := refreshConfig(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

	build, _, err := cfg.Build()
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
			Platform: transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "amd64",
			},
			TrackingChannel: channel.String(),
		}},
	})
}

type composeSuggestionsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&composeSuggestionsSuite{})

func (composeSuggestionsSuite) TestNoReleases(c *gc.C) {
	suggestions := composeSuggestions([]transport.Release{}, params.CharmOrigin{})
	c.Assert(suggestions, gc.DeepEquals, []string(nil))
}

func (composeSuggestionsSuite) TestInvalidPlatform(c *gc.C) {
	suggestions := composeSuggestions([]transport.Release{{
		Platform: "os/series",
	}}, params.CharmOrigin{})
	c.Assert(suggestions, gc.DeepEquals, []string(nil))
}

func (composeSuggestionsSuite) TestNoMatchingArch(c *gc.C) {
	suggestions := composeSuggestions([]transport.Release{{
		Platform: "os/series/arch",
		Channel:  "stable",
	}}, params.CharmOrigin{})
	c.Assert(suggestions, gc.DeepEquals, []string(nil))
}

func (composeSuggestionsSuite) TestSuggestion(c *gc.C) {
	suggestions := composeSuggestions([]transport.Release{{
		Platform: "os/series/arch",
		Channel:  "stable",
	}}, params.CharmOrigin{
		Architecture: "arch",
	})
	c.Assert(suggestions, gc.DeepEquals, []string{
		"stable with arch/os/series",
	})
}

func (composeSuggestionsSuite) TestMultipleSuggestion(c *gc.C) {
	suggestions := composeSuggestions([]transport.Release{{
		Platform: "a/b/c",
		Channel:  "stable",
	}, {
		Platform: "e/f/all",
		Channel:  "2.0/stable",
	}, {
		Platform: "g/h/i",
		Channel:  "stable",
	}}, params.CharmOrigin{
		Architecture: "c",
	})
	c.Assert(suggestions, gc.DeepEquals, []string{
		"stable with c/a/b",
		"2.0/stable with c/e/f",
	})
}

type selectReleaseByChannelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&selectReleaseByChannelSuite{})

func (selectReleaseByChannelSuite) TestNoReleases(c *gc.C) {
	_, err := selectReleaseByArchAndChannel([]transport.Release{}, params.CharmOrigin{})
	c.Assert(err, gc.ErrorMatches, `release not found`)
}

func (selectReleaseByChannelSuite) TestInvalidPlatform(c *gc.C) {
	_, err := selectReleaseByArchAndChannel([]transport.Release{{
		Platform: "os/series",
	}}, params.CharmOrigin{})
	c.Assert(err, gc.ErrorMatches, `release not found`)
}

func (selectReleaseByChannelSuite) TestInvalidChannel(c *gc.C) {
	_, err := selectReleaseByArchAndChannel([]transport.Release{{
		Platform: "os/series/arch",
		Channel:  "",
	}}, params.CharmOrigin{})
	c.Assert(err, gc.ErrorMatches, `release not found`)
}

func (selectReleaseByChannelSuite) TestSelection(c *gc.C) {
	release, err := selectReleaseByArchAndChannel([]transport.Release{{
		Platform: "os/series/arch",
		Channel:  "stable",
	}}, params.CharmOrigin{
		Architecture: "arch",
		Risk:         "stable",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(release, gc.DeepEquals, Release{
		OS:     "os",
		Series: "series",
	})
}

func (selectReleaseByChannelSuite) TestAllSelection(c *gc.C) {
	release, err := selectReleaseByArchAndChannel([]transport.Release{{
		Platform: "os/series/all",
		Channel:  "stable",
	}}, params.CharmOrigin{
		Architecture: "arch",
		Risk:         "stable",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(release, gc.DeepEquals, Release{
		OS:     "os",
		Series: "series",
	})
}

func (selectReleaseByChannelSuite) TestMultipleSelection(c *gc.C) {
	track := "3.0"
	release, err := selectReleaseByArchAndChannel([]transport.Release{{
		Platform: "a/b/c",
		Channel:  "1.0/edge",
	}, {
		Platform: "d/e/all",
		Channel:  "2.0/stable",
	}, {
		Platform: "f/g/h",
		Channel:  "3.0/stable",
	}}, params.CharmOrigin{
		Architecture: "h",
		Track:        &track,
		Risk:         "stable",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(release, gc.DeepEquals, Release{
		OS:     "f",
		Series: "g",
	})
}
