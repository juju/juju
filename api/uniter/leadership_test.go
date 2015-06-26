// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

type leadershipSuite struct {
	testing.IsolationSuite
	stub       *testing.Stub
	responders []responder
	lsa        *uniter.LeadershipSettingsAccessor
}

var _ = gc.Suite(&leadershipSuite{})

type responder func(interface{})

var mockWatcher = struct{ watcher.NotifyWatcher }{}

func (s *leadershipSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.lsa = uniter.NewLeadershipSettingsAccessor(
		func(request string, params, response interface{}) error {
			s.stub.AddCall("FacadeCall", request, params)
			s.nextResponse(response)
			return s.stub.NextErr()
		},
		func(result params.NotifyWatchResult) watcher.NotifyWatcher {
			s.stub.AddCall("NewNotifyWatcher", result)
			return mockWatcher
		},
		func(name string) error {
			s.stub.AddCall("CheckApiVersion", name)
			return s.stub.NextErr()
		},
	)
}

func (s *leadershipSuite) nextResponse(response interface{}) {
	var responder responder
	responder, s.responders = s.responders[0], s.responders[1:]
	if responder != nil {
		responder(response)
	}
}

func (s *leadershipSuite) addResponder(responder responder) {
	s.responders = append(s.responders, responder)
}

func (s *leadershipSuite) CheckCalls(c *gc.C, calls []testing.StubCall, f func()) {
	s.stub = &testing.Stub{}
	s.responders = nil
	f()
	s.stub.CheckCalls(c, calls)
}

func (s *leadershipSuite) TestReadBadVersion(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"Read"},
	}}, func() {
		s.stub.SetErrors(errors.New("splat"))
		settings, err := s.lsa.Read("foobar")
		c.Check(err, gc.ErrorMatches, "cannot access leadership api: splat")
		c.Check(settings, gc.IsNil)
	})
}

func (s *leadershipSuite) expectReadCalls() []testing.StubCall {
	return []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"Read"},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"Read",
			params.Entities{Entities: []params.Entity{{
				Tag: "service-foobar",
			}}},
		},
	}}
}

func (s *leadershipSuite) TestReadSuccess(c *gc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.GetLeadershipSettingsBulkResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.GetLeadershipSettingsResult{{
				Settings: params.Settings{
					"foo": "bar",
					"baz": "qux",
				},
			}}
		})
		settings, err := s.lsa.Read("foobar")
		c.Check(err, jc.ErrorIsNil)
		c.Check(settings, jc.DeepEquals, map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
	})
}

func (s *leadershipSuite) TestReadFailure(c *gc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.GetLeadershipSettingsBulkResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.GetLeadershipSettingsResult{{
				Error: &params.Error{Message: "pow"},
			}}
		})
		settings, err := s.lsa.Read("foobar")
		c.Check(err, gc.ErrorMatches, "failed to read leadership settings: pow")
		c.Check(settings, gc.IsNil)
	})
}

func (s *leadershipSuite) TestReadError(c *gc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(nil, errors.New("blart"))
		settings, err := s.lsa.Read("foobar")
		c.Check(err, gc.ErrorMatches, "failed to call leadership api: blart")
		c.Check(settings, gc.IsNil)
	})
}

func (s *leadershipSuite) TestReadNoResults(c *gc.C) {
	s.CheckCalls(c, s.expectReadCalls(), func() {
		s.addResponder(nil)
		settings, err := s.lsa.Read("foobar")
		c.Check(err, gc.ErrorMatches, "expected 1 result from leadership api, got 0")
		c.Check(settings, gc.IsNil)
	})
}

func (s *leadershipSuite) TestMergeBadVersion(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"Merge"},
	}}, func() {
		s.stub.SetErrors(errors.New("splat"))
		err := s.lsa.Merge("foobar", map[string]string{"foo": "bar"})
		c.Check(err, gc.ErrorMatches, "cannot access leadership api: splat")
	})
}

func (s *leadershipSuite) expectMergeCalls() []testing.StubCall {
	return []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"Merge"},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"Merge",
			params.MergeLeadershipSettingsBulkParams{
				Params: []params.MergeLeadershipSettingsParam{{
					ServiceTag: "service-foobar",
					Settings: map[string]string{
						"foo": "bar",
						"baz": "qux",
					},
				}},
			},
		},
	}}
}

func (s *leadershipSuite) TestMergeSuccess(c *gc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.ErrorResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.ErrorResult{{
				Error: nil,
			}}
		})
		err := s.lsa.Merge("foobar", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *leadershipSuite) TestMergeFailure(c *gc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.ErrorResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.ErrorResult{{
				Error: &params.Error{Message: "zap"},
			}}
		})
		err := s.lsa.Merge("foobar", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, gc.ErrorMatches, "failed to merge leadership settings: zap")
	})
}

func (s *leadershipSuite) TestMergeError(c *gc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(nil, errors.New("dink"))
		err := s.lsa.Merge("foobar", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, gc.ErrorMatches, "failed to call leadership api: dink")
	})
}

func (s *leadershipSuite) TestMergeNoResults(c *gc.C) {
	s.CheckCalls(c, s.expectMergeCalls(), func() {
		s.addResponder(nil)
		err := s.lsa.Merge("foobar", map[string]string{
			"foo": "bar",
			"baz": "qux",
		})
		c.Check(err, gc.ErrorMatches, "expected 1 result from leadership api, got 0")
	})
}

func (s *leadershipSuite) TestWatchBadVersion(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"WatchLeadershipSettings"},
	}}, func() {
		s.stub.SetErrors(errors.New("splat"))
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, gc.ErrorMatches, "cannot access leadership api: splat")
		c.Check(watcher, gc.IsNil)
	})
}

func (s *leadershipSuite) expectWatchCalls() []testing.StubCall {
	return []testing.StubCall{{
		FuncName: "CheckApiVersion",
		Args:     []interface{}{"WatchLeadershipSettings"},
	}, {
		FuncName: "FacadeCall",
		Args: []interface{}{
			"WatchLeadershipSettings",
			params.Entities{Entities: []params.Entity{{
				Tag: "service-foobar",
			}}},
		},
	}}
}

func (s *leadershipSuite) TestWatchSuccess(c *gc.C) {
	expectCalls := append(s.expectWatchCalls(), testing.StubCall{
		FuncName: "NewNotifyWatcher",
		Args: []interface{}{
			params.NotifyWatchResult{
				NotifyWatcherId: "123",
			},
		},
	})
	s.CheckCalls(c, expectCalls, func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.NotifyWatchResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.NotifyWatchResult{{
				NotifyWatcherId: "123",
			}}
		})
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, jc.ErrorIsNil)
		c.Check(watcher, gc.Equals, mockWatcher)
	})
}

func (s *leadershipSuite) TestWatchFailure(c *gc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(func(response interface{}) {
			typed, ok := response.(*params.NotifyWatchResults)
			c.Assert(ok, jc.IsTrue)
			typed.Results = []params.NotifyWatchResult{{
				Error: &params.Error{Message: "blah"},
			}}
		})
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, gc.ErrorMatches, "failed to watch leadership settings: blah")
		c.Check(watcher, gc.IsNil)
	})
}

func (s *leadershipSuite) TestWatchError(c *gc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(nil)
		s.stub.SetErrors(nil, errors.New("snerk"))
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, gc.ErrorMatches, "failed to call leadership api: snerk")
		c.Check(watcher, gc.IsNil)
	})
}

func (s *leadershipSuite) TestWatchNoResults(c *gc.C) {
	s.CheckCalls(c, s.expectWatchCalls(), func() {
		s.addResponder(nil)
		watcher, err := s.lsa.WatchLeadershipSettings("foobar")
		c.Check(err, gc.ErrorMatches, "expected 1 result from leadership api, got 0")
		c.Check(watcher, gc.IsNil)
	})
}
