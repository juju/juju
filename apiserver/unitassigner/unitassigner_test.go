// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestAssignUnits(c *gc.C) {
	f := &fakeState{}
	f.results = []state.UnitAssignmentResult{{Unit: "foo"}}
	api := API{st: f, res: common.NewResources()}
	args := params.AssignUnitsParams{IDs: []string{"foo", "bar"}}
	res, err := api.AssignUnits(args)
	c.Assert(f.ids, gc.DeepEquals, args.IDs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Error, gc.IsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[0].Unit, gc.Equals, "foo")
	c.Assert(res.Results[1].Error, gc.ErrorMatches, `unit "bar" not found`)
	c.Assert(res.Results[1].Unit, gc.Equals, "bar")
}

func (testsuite) TestWatchUnitAssignment(c *gc.C) {
	f := &fakeState{}
	api := API{st: f, res: common.NewResources()}
	f.ids = []string{"boo", "far"}
	res, err := api.WatchUnitAssignments()
	c.Assert(f.watchCalled, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Changes, gc.DeepEquals, f.ids)
}

type fakeState struct {
	watchCalled bool
	ids         []string
	results     []state.UnitAssignmentResult
	err         error
}

func (f *fakeState) WatchForUnitAssignment() state.StringsWatcher {
	f.watchCalled = true
	return fakeWatcher{f.ids}
}

func (f *fakeState) AssignStagedUnits(ids []string) ([]state.UnitAssignmentResult, error) {
	f.ids = ids
	return f.results, f.err
}

type fakeWatcher struct {
	changes []string
}

func (f fakeWatcher) Changes() <-chan []string {
	changes := make(chan []string, 1)
	changes <- f.changes
	return changes
}
func (fakeWatcher) Kill() {}

func (fakeWatcher) Wait() error { return nil }

func (fakeWatcher) Stop() error { return nil }

func (fakeWatcher) Err() error { return nil }
