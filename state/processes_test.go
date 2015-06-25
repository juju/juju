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
