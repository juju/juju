// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type settingsResult struct {
	settings params.Settings
	err      error
}

type RelationCacheSuite struct {
	testhelpers.IsolationSuite
	calls   []string
	results []settingsResult
}

var _ = tc.Suite(&RelationCacheSuite{})

func (s *RelationCacheSuite) SetUpTest(c *tc.C) {
	s.calls = []string{}
	s.results = []settingsResult{}
}

func (s *RelationCacheSuite) ReadSettings(ctx stdcontext.Context, unitName string) (params.Settings, error) {
	result := s.results[len(s.calls)]
	s.calls = append(s.calls, unitName)
	return result.settings, result.err
}

func (s *RelationCacheSuite) TestCreateEmpty(c *tc.C) {
	cache := context.NewRelationCache(s.ReadSettings, nil)
	c.Assert(cache.MemberNames(), tc.HasLen, 0)
	c.Assert(s.calls, tc.HasLen, 0)
}

func (s *RelationCacheSuite) TestCreateWithMembers(c *tc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"u/3", "u/2", "u/1"})
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"u/1", "u/2", "u/3"})
	c.Assert(s.calls, tc.HasLen, 0)
}

func (s *RelationCacheSuite) TestInvalidateMemberChangesMembership(c *tc.C) {
	cache := context.NewRelationCache(s.ReadSettings, nil)
	cache.InvalidateMember("foo/1")
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"foo/1"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"foo/1", "foo/2"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"foo/1", "foo/2"})
	c.Assert(s.calls, tc.HasLen, 0)
}

func (s *RelationCacheSuite) TestRemoveMemberChangesMembership(c *tc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})
	cache.RemoveMember("x/1")
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"x/2"})
	cache.RemoveMember("x/2")
	c.Assert(cache.MemberNames(), tc.HasLen, 0)
	c.Assert(s.calls, tc.HasLen, 0)
}

func (s *RelationCacheSuite) TestPruneChangesMembership(c *tc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"u/1", "u/2", "u/3"})
	cache.Prune([]string{"u/3", "u/4", "u/5"})
	c.Assert(cache.MemberNames(), tc.DeepEquals, []string{"u/3", "u/4", "u/5"})
	c.Assert(s.calls, tc.HasLen, 0)
}

func (s *RelationCacheSuite) TestSettingsPropagatesError(c *tc.C) {
	s.results = []settingsResult{{
		settings: nil, err: errors.New("blam"),
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings(stdcontext.Background(), "whatever/0")
	c.Assert(settings, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "blam")
	c.Assert(s.calls, tc.DeepEquals, []string{"whatever/0"})
}

func (s *RelationCacheSuite) TestSettingsCachesMemberSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	for i := 0; i < 2; i++ {
		settings, err := cache.Settings(stdcontext.Background(), "x/2")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
		c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})
	}
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesMemberSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}, {
		settings: params.Settings{"baz": "qux"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	settings, err := cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesOtherSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}, {
		settings: params.Settings{"baz": "qux"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestRemoveMemberUncachesMemberSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}, {
		settings: params.Settings{"baz": "qux"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	settings, err := cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})

	cache.RemoveMember("x/2")
	settings, err = cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestSettingsCachesOtherSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	for i := 0; i < 2; i++ {
		settings, err := cache.Settings(stdcontext.Background(), "x/2")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
		c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})
	}
}

func (s *RelationCacheSuite) TestPrunePreservesMemberSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"foo/2"})

	settings, err := cache.Settings(stdcontext.Background(), "foo/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"foo/2"})

	cache.Prune([]string{"foo/2"})
	settings, err = cache.Settings(stdcontext.Background(), "foo/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"foo/2"})
}

func (s *RelationCacheSuite) TestPruneUncachesOtherSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}, {
		settings: params.Settings{"baz": "qux"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2"})

	cache.Prune(nil)
	settings, err = cache.Settings(stdcontext.Background(), "x/2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestApplicationSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)
	settings, err := cache.ApplicationSettings(stdcontext.Background(), "x")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x"})
}

func (s *RelationCacheSuite) TestInvalidateApplicationSettings(c *tc.C) {
	s.results = []settingsResult{{
		settings: params.Settings{"foo": "bar"}, err: nil,
	}, {
		settings: params.Settings{"foo": "baz"}, err: nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)
	settings, err := cache.ApplicationSettings(stdcontext.Background(), "x")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x"})

	// Calling it a second time returns the value from the cache
	settings, err = cache.ApplicationSettings(stdcontext.Background(), "x")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x"})

	// Now when we Invalidate the application, it will read it again
	cache.InvalidateApplication("x")
	settings, err = cache.ApplicationSettings(stdcontext.Background(), "x")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(settings, tc.DeepEquals, params.Settings{"foo": "baz"})
	c.Assert(s.calls, tc.DeepEquals, []string{"x", "x"})
}
