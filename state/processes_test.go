// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&processDefinitionsSuite{})
var _ = gc.Suite(&unitProcessesSuite{})

type baseProcessesSuite struct {
	testing.BaseSuite

	stub    *gitjujutesting.Stub
	persist *fakeProcsPersistence
	charm   names.CharmTag
	unit    names.UnitTag
}

func (s *baseProcessesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.persist = &fakeProcsPersistence{Stub: s.stub}
	s.charm = names.NewCharmTag("local:series/dummy-1")
	s.unit = names.NewUnitTag("a-unit/0")
}

func (s *baseProcessesSuite) setUnit(id string) {
	if id == "" {
		s.unit = names.UnitTag{}
	} else {
		s.unit = names.NewUnitTag(id)
	}
}

func (s *baseProcessesSuite) setCharm(id string) {
	if id == "" {
		s.charm = names.CharmTag{}
	} else {
		s.charm = names.NewCharmTag(id)
	}
}

func (s *baseProcessesSuite) newDefinitions(pType string, names ...string) []charm.Process {
	var definitions []charm.Process
	for _, name := range names {
		definitions = append(definitions, charm.Process{
			Name: name,
			Type: pType,
		})
	}
	return definitions
}

func (s *baseProcessesSuite) newProcesses(pType string, names ...string) []process.Info {
	var ids []string
	for i, name := range names {
		name, id := process.ParseID(name)
		names[i] = name
		if id == "" {
			id = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
		}
		ids = append(ids, id)
	}

	var processes []process.Info
	for i, definition := range s.newDefinitions(pType, names...) {
		id := ids[i]
		processes = append(processes, process.Info{
			Process: definition,
			Details: process.Details{
				ID: id,
				Status: process.Status{
					Label: "running",
				},
			},
		})
	}
	return processes
}

type processDefinitionsSuite struct {
	baseProcessesSuite
}

func (s *processDefinitionsSuite) TestEnsureDefinedOkay(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA", "procB")
	pd := state.ProcessDefinitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnsureDefinitions")
	s.persist.checkDefinitions(c, definitions)
}

func (s *processDefinitionsSuite) TestEnsureDefinedNoOp(c *gc.C) {
	pd := state.ProcessDefinitions{Persist: s.persist}
	err := pd.EnsureDefined()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnsureDefinitions")
	c.Check(s.persist.definitions, gc.HasLen, 0)
}

func (s *processDefinitionsSuite) TestEnsureDefinedBadDefinition(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA", "procB")
	definitions = append(definitions, charm.Process{})
	pd := state.ProcessDefinitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *processDefinitionsSuite) TestEnsureDefinedMatched(c *gc.C) {
	same := charm.Process{Name: "procA", Type: "docker"}
	s.persist.setDefinitions(&same)

	definitions := s.newDefinitions("docker", "procA", "procB")
	pd := state.ProcessDefinitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	s.persist.checkDefinitions(c, definitions)
}

func (s *processDefinitionsSuite) TestEnsureDefinedMismatched(c *gc.C) {
	same := charm.Process{Name: "procA", Type: "docker"}
	different := charm.Process{Name: "procB", Type: "kvm"}
	s.persist.setDefinitions(&same, &different)

	definitions := s.newDefinitions("docker", "procA", "procB", "procC")
	definitions = append(definitions, same)
	pd := state.ProcessDefinitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

type unitProcessesSuite struct {
	baseProcessesSuite
}

func (s *unitProcessesSuite) TestRegisterOkay(c *gc.C) {
	procs := s.newProcesses("docker", "procA")
	proc := procs[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnsureDefinitions", "Insert")
	s.persist.checkProcesses(c, procs)
}

func (s *unitProcessesSuite) TestRegisterInvalid(c *gc.C) {
	proc := s.newProcesses("", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitProcessesSuite) TestRegisterEnsureDefinitionFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestRegisterMismatchedDefinition(c *gc.C) {
	s.persist.setDefinitions(&charm.Process{Name: "procA", Type: "kvm"})
	proc := s.newProcesses("docker", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitProcessesSuite) TestRegisterInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(nil, failure)
	proc := s.newProcesses("docker", "procA")[0]

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestRegisterAlreadyExists(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Register(proc, s.charm)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitProcessesSuite) TestSetStatusOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)
	status := process.Status{
		Label: "okay",
	}

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus(proc.ID(), status)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStatus")
	c.Check(s.persist.procs[proc.ID()].Details.Status, jc.DeepEquals, status)
}

func (s *unitProcessesSuite) TestSetStatusFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)
	status := process.Status{
		Label: "okay",
	}

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus(proc.ID(), status)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitProcessesSuite) TestSetStatusMissing(c *gc.C) {
	status := process.Status{
		Label: "okay",
	}

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.SetStatus("some-proc", status)

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

func (s *unitProcessesSuite) TestUnregisterOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA")[0]
	s.persist.setProcesses(&proc)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Unregister(proc.ID())
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Remove")
	c.Check(s.persist.procs, gc.HasLen, 0)
}

func (s *unitProcessesSuite) TestUnregisterMissing(c *gc.C) {
	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Unregister("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Remove")
	c.Check(s.persist.procs, gc.HasLen, 0)
}

func (s *unitProcessesSuite) TestUnregisterFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitProcesses{Persist: s.persist}
	err := ps.Unregister("procA/xyz")

	s.stub.CheckCallNames(c, "Remove")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

type fakeProcsPersistence struct {
	*gitjujutesting.Stub
	definitions  map[string]*charm.Process
	procs        map[string]*process.Info
	inconsistent []string
}

func (s *fakeProcsPersistence) checkDefinitions(c *gc.C, expectedList []charm.Process) {
	c.Check(s.definitions, gc.HasLen, len(expectedList))
	for _, expected := range expectedList {
		definition, ok := s.definitions[expected.Name]
		if !ok {
			c.Errorf("definition %q not found", expected.Name)
		} else {
			c.Check(definition, jc.DeepEquals, &expected)
		}
	}
}

func (s *fakeProcsPersistence) checkProcesses(c *gc.C, expectedList []process.Info) {
	c.Check(s.procs, gc.HasLen, len(expectedList))
	for _, expected := range expectedList {
		proc, ok := s.procs[expected.ID()]
		if !ok {
			c.Errorf("process %q not found", expected.ID())
		} else {
			c.Check(proc, jc.DeepEquals, &expected)
		}

		definition, ok := s.definitions[expected.Name]
		if !ok {
			c.Errorf("definition %q not found", expected.Name)
		} else {
			c.Check(definition, jc.DeepEquals, &expected.Process)
		}
	}
}

func (s *fakeProcsPersistence) setDefinitions(definitions ...*charm.Process) {
	if s.definitions == nil {
		s.definitions = make(map[string]*charm.Process)
	}
	for _, definition := range definitions {
		s.definitions[definition.Name] = definition
	}
}

func (s *fakeProcsPersistence) setProcesses(procs ...*process.Info) {
	if s.procs == nil {
		s.procs = make(map[string]*process.Info)
	}
	for _, proc := range procs {
		s.procs[proc.ID()] = proc
	}
}

func (s *fakeProcsPersistence) EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error) {
	s.AddCall("EnsureDefinitions", definitions)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var existing []string
	var mismatched []string
	for _, definition := range definitions {
		if added, ok := s.ensureDefinition(definition); !added {
			existing = append(existing, definition.Name)
			if !ok {
				mismatched = append(mismatched, definition.Name)
			}
		}
	}
	return existing, mismatched, nil
}

func (s *fakeProcsPersistence) ensureDefinition(definition charm.Process) (bool, bool) {
	if expected, ok := s.definitions[definition.Name]; ok {
		if !reflect.DeepEqual(&definition, expected) {
			return false, false
		}
		return false, true
	} else {
		s.setDefinitions(&definition)
		return true, true
	}
}

func (s *fakeProcsPersistence) Insert(info process.Info) (bool, error) {
	s.AddCall("Insert", info)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.procs[info.ID()]; ok {
		return false, nil
	}
	s.setProcesses(&info)
	return true, nil
}

func (s *fakeProcsPersistence) SetStatus(id string, status process.Status) (bool, error) {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	proc, ok := s.procs[id]
	if !ok {
		return false, nil
	}
	proc.Details.Status = status
	return true, nil
}

func (s *fakeProcsPersistence) List(ids ...string) ([]process.Info, []string, error) {
	s.AddCall("List", ids)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var procs []process.Info
	var missing []string
	for _, id := range ids {
		if proc, ok := s.procs[id]; !ok {
			missing = append(missing, id)
		} else {
			for _, inconsistent := range s.inconsistent {
				if id == inconsistent {
					return nil, nil, errors.NotValidf(id)
				}
			}
			procs = append(procs, *proc)
		}
	}
	return procs, missing, nil
}

func (s *fakeProcsPersistence) ListAll() ([]process.Info, error) {
	s.AddCall("ListAll")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var procs []process.Info
	for _, proc := range s.procs {
		procs = append(procs, *proc)
	}
	return procs, nil
}

func (s *fakeProcsPersistence) Remove(id string) (bool, error) {
	s.AddCall("Remove", id)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.procs[id]; !ok {
		return false, nil
	}
	for _, inconsistent := range s.inconsistent {
		if id == inconsistent {
			return false, errors.NotValidf(id)
		}
	}
	delete(s.procs, id)
	// TODO(ericsnow) Remove definition if appropriate.
	return true, nil
}
