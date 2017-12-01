// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

type settingsResult struct {
	settings commands.Settings
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

func (s *RelationCacheSuite) RemoteSettings(unitName string) (commands.Settings, error) {
	result := s.results[len(s.calls)]
	s.calls = append(s.calls, unitName)
	return result.settings, result.err
}

func (s *RelationCacheSuite) TestCreateEmpty(c *gc.C) {
	cache := context.NewRelationCache(s.RemoteSettings, nil)
	c.Assert(cache.MemberNames(), gc.HasLen, 0)
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestCreateWithMembers(c *gc.C) {
	cache := context.NewRelationCache(s.RemoteSettings, []string{"u/3", "u/2", "u/1"})
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"u/1", "u/2", "u/3"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestInvalidateMemberChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.RemoteSettings, nil)
	cache.InvalidateMember("foo/1")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1", "foo/2"})
	cache.InvalidateMember("foo/2")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"foo/1", "foo/2"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestRemoveMemberChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.RemoteSettings, []string{"x/2"})
	cache.RemoveMember("x/1")
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"x/2"})
	cache.RemoveMember("x/2")
	c.Assert(cache.MemberNames(), gc.HasLen, 0)
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestPruneChangesMembership(c *gc.C) {
	cache := context.NewRelationCache(s.RemoteSettings, []string{"u/1", "u/2", "u/3"})
	cache.Prune([]string{"u/3", "u/4", "u/5"})
	c.Assert(cache.MemberNames(), jc.DeepEquals, []string{"u/3", "u/4", "u/5"})
	c.Assert(s.calls, gc.HasLen, 0)
}

func (s *RelationCacheSuite) TestSettingsPropagatesError(c *gc.C) {
	s.results = []settingsResult{{
		nil, errors.New("blam"),
	}}
	cache := context.NewRelationCache(s.RemoteSettings, nil)

	settings, err := cache.Settings("whatever")
	c.Assert(settings, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "blam")
	c.Assert(s.calls, jc.DeepEquals, []string{"whatever"})
}

func (s *RelationCacheSuite) TestSettingsCachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, []string{"x/2"})

	for i := 0; i < 2; i++ {
		settings, err := cache.Settings("x/2")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
		c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})
	}
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}, {
		runnertesting.Settings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, []string{"x/2"})

	settings, err := cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestInvalidateMemberUncachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}, {
		runnertesting.Settings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, nil)

	settings, err := cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.InvalidateMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestRemoveMemberUncachesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}, {
		runnertesting.Settings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, []string{"x/2"})

	settings, err := cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.RemoveMember("x/2")
	settings, err = cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}

func (s *RelationCacheSuite) TestSettingsCachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, nil)

	for i := 0; i < 2; i++ {
		settings, err := cache.Settings("x/2")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
		c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})
	}
}

func (s *RelationCacheSuite) TestPrunePreservesMemberSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, []string{"foo/2"})

	settings, err := cache.Settings("foo/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"foo/2"})

	cache.Prune([]string{"foo/2"})
	settings, err = cache.Settings("foo/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"foo/2"})
}

func (s *RelationCacheSuite) TestPruneUncachesOtherSettings(c *gc.C) {
	s.results = []settingsResult{{
		runnertesting.Settings{"foo": "bar"}, nil,
	}, {
		runnertesting.Settings{"baz": "qux"}, nil,
	}}
	cache := context.NewRelationCache(s.RemoteSettings, nil)

	settings, err := cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"foo": "bar"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2"})

	cache.Prune(nil)
	settings, err = cache.Settings("x/2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, runnertesting.Settings{"baz": "qux"})
	c.Assert(s.calls, jc.DeepEquals, []string{"x/2", "x/2"})
}
