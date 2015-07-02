// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process/state"
)

var _ = gc.Suite(&processDefinitionsSuite{})

type processDefinitionsSuite struct {
	baseProcessesSuite
}

func (s *processDefinitionsSuite) TestEnsureDefinedOkay(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA", "procB")
	pd := state.Definitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnsureDefinitions")
	s.persist.checkDefinitions(c, definitions)
}

func (s *processDefinitionsSuite) TestEnsureDefinedNoOp(c *gc.C) {
	pd := state.Definitions{Persist: s.persist}
	err := pd.EnsureDefined()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnsureDefinitions")
	c.Check(s.persist.definitions, gc.HasLen, 0)
}

func (s *processDefinitionsSuite) TestEnsureDefinedBadDefinition(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA", "procB")
	definitions = append(definitions, charm.Process{})
	pd := state.Definitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *processDefinitionsSuite) TestEnsureDefinedMatched(c *gc.C) {
	same := charm.Process{Name: "procA", Type: "docker"}
	s.persist.setDefinitions(&same)

	definitions := s.newDefinitions("docker", "procA", "procB")
	pd := state.Definitions{Persist: s.persist}
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
	pd := state.Definitions{Persist: s.persist}
	err := pd.EnsureDefined(definitions...)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}
