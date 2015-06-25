// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
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

func (s *processDefinitionsSuite) TestEnsureDefined(c *gc.C) {
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
