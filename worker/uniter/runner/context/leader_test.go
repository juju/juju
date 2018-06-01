// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type LeaderSuite struct {
	testing.IsolationSuite
	testing.Stub
	accessor *StubLeadershipSettingsAccessor
	tracker  *StubTracker
	context  context.LeadershipContext
}

var _ = gc.Suite(&LeaderSuite{})

func (s *LeaderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.accessor = &StubLeadershipSettingsAccessor{
		Stub: &s.Stub,
	}
	s.tracker = &StubTracker{
		Stub:            &s.Stub,
		applicationName: "led-application",
	}
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ApplicationName",
	}}, func() {
		s.context = context.NewLeadershipContext(s.accessor, s.tracker, "u/0")
	})
}

func (s *LeaderSuite) CheckCalls(c *gc.C, stubCalls []testing.StubCall, f func()) {
	s.Stub = testing.Stub{}
	f()
	s.Stub.CheckCalls(c, stubCalls)
}

func (s *LeaderSuite) TestIsLeaderSuccess(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// ...and so does the second.
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailure(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// ...and the second doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestIsLeaderFailureAfterSuccess(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call succeeds...
		s.tracker.results = []StubTicket{true}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsTrue)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The second fails...
		s.tracker.results = []StubTicket{false}
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The third doesn't even try.
		leader, err := s.context.IsLeader()
		c.Check(leader, jc.IsFalse)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestLeaderSettingsSuccess(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		// The first call grabs the settings...
		s.accessor.results = []map[string]string{{
			"some": "settings",
			"of":   "interest",
		}}
		settings, err := s.context.LeaderSettings()
		c.Check(settings, jc.DeepEquals, map[string]string{
			"some": "settings",
			"of":   "interest",
		})
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, nil, func() {
		// The second uses the cache.
		settings, err := s.context.LeaderSettings()
		c.Check(settings, jc.DeepEquals, map[string]string{
			"some": "settings",
			"of":   "interest",
		})
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestLeaderSettingsCopyMap(c *gc.C) {
	// Grab the settings to populate the cache...
	s.accessor.results = []map[string]string{{
		"some": "settings",
		"of":   "interest",
	}}
	settings, err := s.context.LeaderSettings()
	c.Check(err, gc.IsNil)

	// Put some nonsense into the returned settings...
	settings["bad"] = "news"

	// Get the settings again and check they're as expected.
	settings, err = s.context.LeaderSettings()
	c.Check(settings, jc.DeepEquals, map[string]string{
		"some": "settings",
		"of":   "interest",
	})
	c.Check(err, jc.ErrorIsNil)
}

func (s *LeaderSuite) TestLeaderSettingsError(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		s.accessor.results = []map[string]string{nil}
		s.Stub.SetErrors(errors.New("blort"))
		settings, err := s.context.LeaderSettings()
		c.Check(settings, gc.IsNil)
		c.Check(err, gc.ErrorMatches, "cannot read settings: blort")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsSuccess(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		s.tracker.results = []StubTicket{true}
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsMinion(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}}, func() {
		// The first call fails...
		s.tracker.results = []StubTicket{false}
		err := s.context.WriteLeaderSettings(map[string]string{"blah": "blah"})
		c.Check(err, gc.ErrorMatches, "cannot write settings: not the leader")
	})

	s.CheckCalls(c, nil, func() {
		// The second doesn't even try.
		err := s.context.WriteLeaderSettings(map[string]string{"blah": "blah"})
		c.Check(err, gc.ErrorMatches, "cannot write settings: not the leader")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsError(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		s.tracker.results = []StubTicket{true}
		s.Stub.SetErrors(errors.New("glurk"))
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, gc.ErrorMatches, "cannot write settings: glurk")
	})
}

func (s *LeaderSuite) TestWriteLeaderSettingsClearsCache(c *gc.C) {
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		// Start off by populating the cache...
		s.accessor.results = []map[string]string{{
			"some": "settings",
			"of":   "interest",
		}}
		_, err := s.context.LeaderSettings()
		c.Check(err, gc.IsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "ClaimLeader",
	}, {
		FuncName: "Merge",
		Args: []interface{}{"led-application", "u/0", map[string]string{
			"some": "very",
			"nice": "data",
		}},
	}}, func() {
		// Write new data to the controller...
		s.tracker.results = []StubTicket{true}
		err := s.context.WriteLeaderSettings(map[string]string{
			"some": "very",
			"nice": "data",
		})
		c.Check(err, jc.ErrorIsNil)
	})

	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "Read",
		Args:     []interface{}{"led-application"},
	}}, func() {
		s.accessor.results = []map[string]string{{
			"totally": "different",
			"server":  "decides",
		}}
		settings, err := s.context.LeaderSettings()
		c.Check(err, gc.IsNil)
		c.Check(settings, jc.DeepEquals, map[string]string{
			"totally": "different",
			"server":  "decides",
		})
		c.Check(err, jc.ErrorIsNil)
	})
}

type StubLeadershipSettingsAccessor struct {
	*testing.Stub
	results []map[string]string
}

func (stub *StubLeadershipSettingsAccessor) Read(applicationName string) (result map[string]string, _ error) {
	stub.MethodCall(stub, "Read", applicationName)
	result, stub.results = stub.results[0], stub.results[1:]
	return result, stub.NextErr()
}

func (stub *StubLeadershipSettingsAccessor) Merge(applicationName, unitName string, settings map[string]string) error {
	stub.MethodCall(stub, "Merge", applicationName, unitName, settings)
	return stub.NextErr()
}

type StubTracker struct {
	leadership.Tracker
	*testing.Stub
	applicationName string
	results         []StubTicket
}

func (stub *StubTracker) ApplicationName() string {
	stub.MethodCall(stub, "ApplicationName")
	return stub.applicationName
}

func (stub *StubTracker) ClaimLeader() (result leadership.Ticket) {
	stub.MethodCall(stub, "ClaimLeader")
	result, stub.results = stub.results[0], stub.results[1:]
	return result
}

type StubTicket bool

func (ticket StubTicket) Wait() bool {
	return bool(ticket)
}

func (ticket StubTicket) Ready() <-chan struct{} {
	return alwaysReady
}

var alwaysReady = make(chan struct{})

func init() {
	close(alwaysReady)
}
