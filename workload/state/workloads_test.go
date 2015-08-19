// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/state"
)

var _ = gc.Suite(&unitProcessesSuite{})

type unitProcessesSuite struct {
	baseProcessesSuite
}

func (s *unitProcessesSuite) TestAddOkay(c *gc.C) {
	procs := s.newProcesses("docker", "procA")
	proc := procs[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Add(proc)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Insert")
	s.persist.checkProcesses(c, procs)
}

func (s *unitProcessesSuite) TestAddInvalid(c *gc.C) {
	proc := s.newProcesses("", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Add(proc)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitProcessesSuite) TestAddEnsureDefinitionFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Add(proc)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestAddInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Add(proc)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestAddAlreadyExists(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Add(proc)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func newStatusInfo(state, message, pluginStatus string) process.CombinedStatus {
	return process.CombinedStatus{
		Status: process.Status{
			State:   state,
			Message: message,
		},
		PluginStatus: process.PluginStatus{
			State: pluginStatus,
		},
	}
}

func (s *unitProcessesSuite) TestSetStatusOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)
	status := newStatusInfo(process.StateRunning, "good to go", "okay")

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus(proc.ID(), status)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStatus")
	current := s.persist.procs[proc.ID()]
	c.Check(current.Status, jc.DeepEquals, status.Status)
	c.Check(current.Details.Status, jc.DeepEquals, status.PluginStatus)
}

func (s *unitProcessesSuite) TestSetStatusFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)
	status := newStatusInfo(process.StateRunning, "good to go", "okay")

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus(proc.ID(), status)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestSetStatusMissing(c *gc.C) {
	status := newStatusInfo(process.StateRunning, "good to go", "okay")

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus("some/proc", status)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitProcessesSuite) TestListOkay(c *gc.C) {
	proc1 := s.newProcesses("docker", "procA")[0]
	proc2 := s.newProcesses("docker", "procB")[0]
	s.persist.setProcesses(&proc1, &proc2)

	ps := state.UnitProcesses{Persist: s.persist}
	procs, err := ps.List(proc1.ID())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "List")
	c.Check(procs, jc.DeepEquals, []process.Info{proc1})
}

func (s *unitProcessesSuite) TestListAll(c *gc.C) {
	expected := s.newProcesses("docker", "procA", "procB")
	s.persist.setProcesses(&expected[0], &expected[1])

	ps := state.UnitProcesses{Persist: s.persist}
	procs, err := ps.List()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(procs, gc.HasLen, 2)
	if procs[0].Name == "procA" {
		c.Check(procs[0], jc.DeepEquals, expected[0])
		c.Check(procs[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(procs[0], jc.DeepEquals, expected[1])
		c.Check(procs[1], jc.DeepEquals, expected[0])
	}
}

func (s *unitProcessesSuite) TestListFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitProcesses{Persist: s.persist}
	_, err := ps.List()

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestListMissing(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)

	ps := state.UnitProcesses{Persist: s.persist}
	procs, err := ps.List(proc.ID(), "procB/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, jc.DeepEquals, []process.Info{proc})
}

func (s *unitProcessesSuite) TestListDefinitions(c *gc.C) {
	expected := s.newProcesses("docker", "procA", "procB")
	getMetadata := func() (*charm.Meta, error) {
		meta := &charm.Meta{
			Processes: map[string]charm.Process{
				"procA": expected[0].Process,
				"procB": expected[1].Process,
			},
		}
		return meta, nil
	}
	ps := state.UnitProcesses{Persist: s.persist}
	ps.Metadata = getMetadata

	definitions, err := ps.ListDefinitions()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, nil)
	c.Check(definitions, gc.HasLen, 2)
	if definitions[0].Name == "procA" {
		c.Check(definitions[0], jc.DeepEquals, expected[0].Process)
		c.Check(definitions[1], jc.DeepEquals, expected[1].Process)
	} else {
		c.Check(definitions[0], jc.DeepEquals, expected[1].Process)
		c.Check(definitions[1], jc.DeepEquals, expected[0].Process)
	}
}

func (s *unitProcessesSuite) TestRemoveOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Remove(proc.ID())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Remove")
	c.Check(s.persist.procs, gc.HasLen, 0)
}

func (s *unitProcessesSuite) TestRemoveMissing(c *gc.C) {
	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Remove")
	c.Check(s.persist.procs, gc.HasLen, 0)
}

func (s *unitProcessesSuite) TestRemoveFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Remove("procA/xyz")

	s.stub.CheckCallNames(c, "Remove")
	c.Check(errors.Cause(err), gc.Equals, failure)
}
