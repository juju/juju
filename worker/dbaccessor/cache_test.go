// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type cacheSuite struct {
	testing.IsolationSuite

	clock *MockClock
}

var _ = gc.Suite(&cacheSuite{})

func (s *cacheSuite) TestSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	tests := []struct {
		set    string
		known  bool
		has    string
		exists bool
	}{
		{set: "foo", known: true, has: "foo", exists: true},
		{set: "foo", known: false, has: "foo", exists: false},
		{set: "", known: false, has: "foo", exists: false},
	}

	for _, test := range tests {
		cache := newNSCache(time.Second, s.clock)
		if test.set != "" {
			cache.Set(test.set, test.exists)
		}

		c.Assert(cache.Exists(test.has), gc.Equals, test.exists)
	}
}

func (s *cacheSuite) TestExistsExpired(c *gc.C) {
	defer s.setupMocks(c).Finish()

	gomock.InOrder(
		s.clock.EXPECT().Now().Return(time.Now()),
		s.clock.EXPECT().Now().Return(time.Now()),
		s.clock.EXPECT().Now().Return(time.Now().Add(time.Second*2)),
	)

	cache := newNSCache(time.Second, s.clock)
	cache.Set("foo", true)
	c.Assert(cache.Exists("foo"), jc.IsTrue)
	c.Assert(cache.Exists("foo"), jc.IsFalse)
}

func (s *cacheSuite) TestFlush(c *gc.C) {
	defer s.setupMocks(c).Finish()

	gomock.InOrder(
		s.clock.EXPECT().Now().Return(time.Now()),
		s.clock.EXPECT().Now().Return(time.Now()),
		s.clock.EXPECT().Now().Return(time.Now().Add(time.Second*2)),
	)

	cache := newNSCache(time.Second, s.clock)
	cache.Set("foo", true)
	c.Assert(cache.Exists("foo"), jc.IsTrue)

	cache.Flush()
	c.Assert(cache.Exists("foo"), jc.IsFalse)
}

func (s *cacheSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}
