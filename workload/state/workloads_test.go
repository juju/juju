// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/state"
)

var _ = gc.Suite(&unitWorkloadsSuite{})

type unitWorkloadsSuite struct {
	baseWorkloadsSuite
}

func (s *unitWorkloadsSuite) TestTrackOkay(c *gc.C) {
	workloads := s.newWorkloads("docker", "workloadA")
	wl := workloads[0]

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Track(wl)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Track")
	s.persist.checkWorkloads(c, workloads)
}

func (s *unitWorkloadsSuite) TestTrackInvalid(c *gc.C) {
	wl := s.newWorkloads("", "workloadA")[0]

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Track(wl)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitWorkloadsSuite) TestTrackEnsureDefinitionFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	wl := s.newWorkloads("docker", "wlA")[0]

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Track(wl)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitWorkloadsSuite) TestTrackInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	wl := s.newWorkloads("docker", "workloadA")[0]

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Track(wl)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitWorkloadsSuite) TestTrackAlreadyExists(c *gc.C) {
	wl := s.newWorkloads("docker", "workloadA")[0]
	s.persist.setWorkloads(&wl)

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Track(wl)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func newStatusInfo(state, message, pluginStatus string) workload.CombinedStatus {
	return workload.CombinedStatus{
		Status: workload.Status{
			State:   state,
			Message: message,
		},
		PluginStatus: workload.PluginStatus{
			State: pluginStatus,
		},
	}
}

func (s *unitWorkloadsSuite) TestSetStatusOkay(c *gc.C) {
	wl := s.newWorkloads("docker", "workloadA")[0]
	s.persist.setWorkloads(&wl)

	ps := state.UnitWorkloads{Persist: s.persist}

	cls, id := workload.ParseID(wl.ID())
	err := ps.SetStatus(cls, id, workload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStatus")
	current := s.persist.workloads[wl.ID()]
	c.Check(current.Status.State, jc.DeepEquals, workload.StateRunning)
	c.Check(current.Details.Status.State, jc.DeepEquals, workload.StateRunning)
}

func (s *unitWorkloadsSuite) TestSetStatusFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	wl := s.newWorkloads("docker", "workloadA")[0]
	s.persist.setWorkloads(&wl)

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.SetStatus("docker", "workloadA", workload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitWorkloadsSuite) TestSetStatusMissing(c *gc.C) {
	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.SetStatus("some", "workload", workload.StateRunning)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitWorkloadsSuite) TestListOkay(c *gc.C) {
	wl1 := s.newWorkloads("docker", "workloadA")[0]
	wl2 := s.newWorkloads("docker", "workloadB")[0]
	s.persist.setWorkloads(&wl1, &wl2)

	ps := state.UnitWorkloads{Persist: s.persist}
	workloads, err := ps.List(wl1.ID())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "List")
	c.Check(workloads, jc.DeepEquals, []workload.Info{wl1})
}

func (s *unitWorkloadsSuite) TestListAll(c *gc.C) {
	expected := s.newWorkloads("docker", "workloadA", "workloadB")
	s.persist.setWorkloads(&expected[0], &expected[1])

	ps := state.UnitWorkloads{Persist: s.persist}
	workloads, err := ps.List()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(workloads, gc.HasLen, 2)
	if workloads[0].Name == "workloadA" {
		c.Check(workloads[0], jc.DeepEquals, expected[0])
		c.Check(workloads[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(workloads[0], jc.DeepEquals, expected[1])
		c.Check(workloads[1], jc.DeepEquals, expected[0])
	}
}

func (s *unitWorkloadsSuite) TestListFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitWorkloads{Persist: s.persist}
	_, err := ps.List()

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitWorkloadsSuite) TestListMissing(c *gc.C) {
	wl := s.newWorkloads("docker", "workloadA")[0]
	s.persist.setWorkloads(&wl)

	ps := state.UnitWorkloads{Persist: s.persist}
	workloads, err := ps.List(wl.ID(), "workloadB/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(workloads, jc.DeepEquals, []workload.Info{wl})
}

func (s *unitWorkloadsSuite) TestUntrackOkay(c *gc.C) {
	wl := s.newWorkloads("docker", "workloadA")[0]
	s.persist.setWorkloads(&wl)

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Untrack(wl.ID())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.workloads, gc.HasLen, 0)
}

func (s *unitWorkloadsSuite) TestUntrackMissing(c *gc.C) {
	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Untrack("workloadA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.workloads, gc.HasLen, 0)
}

func (s *unitWorkloadsSuite) TestUntrackFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitWorkloads{Persist: s.persist}
	err := ps.Untrack("workloadA/xyz")

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(errors.Cause(err), gc.Equals, failure)
}
