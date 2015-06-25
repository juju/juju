// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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
	var processes []process.Info
	for _, definition := range s.newDefinitions(pType, names...) {
		processes = append(processes, process.Info{
			Process: definition,
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

func (s *unitProcessesSuite) TestRegister(c *gc.C) {
}

func (s *unitProcessesSuite) TestSetStatus(c *gc.C) {
}

func (s *unitProcessesSuite) TestList(c *gc.C) {
}

func (s *unitProcessesSuite) TestUnregister(c *gc.C) {
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

func (s *fakeProcsPersistence) SetStatus(id string, status process.RawStatus) (bool, error) {
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
