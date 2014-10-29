// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/context"
)

type settingsResult struct {
	settings params.RelationSettings
	err      error
}

type RelationCacheSuite struct {
	testing.IsolationSuite
	calls   []string
	results []settingsResult
}

var _ = gc.Suite(&RelationCacheSuite{})

func (s *RelationCacheSuite) SetUpTest(c *gc.C) {
	s.calls = []string{}
	s.results = []settingsResult{}
}

func (s *RelationCacheSuite) ReadSettings(unitName string) (params.RelationSettings, error) {
	result := s.results[len(s.calls)]
	s.calls = append(s.calls, unitName)
	return result.settings, result.err
}

func (s *RelationCacheSuite) TestCreateEmpty(c *gc.C) {
	cache := context.NewRelationCache(s.ReadSettings, nil)
	c.Assert(cache.MemberNames(), gc.HasLen, 0)
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestCreateWithMembers(c *gc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"u/1", "u/2", "u/3"})
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"u/1", "u/2", "u/3"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestInvalidateMemberChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.ReadSettings, nil)
	cache.InvalidateMember("foo/1")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1", "foo/2"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1", "foo/2"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestRemoveMemberChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})
	cache.RemoveMember("x/1")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"x/2"})
	cache.RemoveMember("x/2")
	c.Assert(cache.MemberNames(), gc.HasLen, 0)
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestPruneChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.ReadSettings, []string{"u/1", "u/2", "u/3"})
	cache.Prune([]string{"u/3", "u/4", "u/5"})
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"u/3", "u/4", "u/5"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestSettingsPropagatesError(c *gc.C) {
	s.results = []settingsResult{{
		nil, fmt.Errorf("blam"),
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings("whatever")
	c.Assert(settings, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(s.calls, jc.DeepEquals, []string{"whatever"})
}

func (s *RelationCacheSuite) TestSettingsCachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}, {
		params.RelationSettings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}, {
		params.RelationSettings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestRemoveMemberUncachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}, {
		params.RelationSettings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"x/2"})

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.RemoveMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestSettingsCachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})
}

func (s *RelationCacheSuite) TestPrunePreservesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, []string{"foo/2"})

	settings, err := cache.Settings("foo/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"foo/2"})

	cache.Prune([]string{"foo/2"})
	settings, err = cache.Settings("foo/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"foo/2"})
}

func (s *RelationCacheSuite) TestPruneUncachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		params.RelationSettings{"foo": "bar"}, nil,
	}, {
		params.RelationSettings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.ReadSettings, nil)

	settings, err := cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.Prune(nil)
	settings, err = cache.Settings("x/2")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}
