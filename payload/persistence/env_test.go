// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&envPersistenceSuite{})

type envPersistenceSuite struct {
	BaseSuite

	base *stubEnvPersistenceBase
}

func (s *envPersistenceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.base = &stubEnvPersistenceBase{
		PersistenceBase: s.State,
		stub:            s.Stub,
	}
}

func (s *envPersistenceSuite) newPayload(name string) payload.FullPayloadInfo {
	return payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: payload.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	}
}

func (s *envPersistenceSuite) TestListAllOkay(c *gc.C) {
	s.base.setUnits("0")
	s.base.setUnits("1", "a-service/0", "a-service/1")
	s.base.setUnits("2", "a-service/2")
	p1 := s.newPayload("spam")
	p2 := s.newPayload("eggs")
	s.base.setPayloads(p1, p2)

	persist := NewEnvPersistence(s.base)
	persist.newUnitPersist = s.base.newUnitPersistence

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, p1, p2)
	s.Stub.CheckCallNames(c,
		"Machines",

		"MachineUnits",

		"MachineUnits",
		"newUnitPersistence",
		"ListAll",
		"newUnitPersistence",
		"ListAll",

		"MachineUnits",
		"newUnitPersistence",
		"ListAll",
	)
}

func (s *envPersistenceSuite) TestListAllEmpty(c *gc.C) {
	s.base.setUnits("0")
	s.base.setUnits("1", "a-service/0", "a-service/1")
	persist := NewEnvPersistence(s.base)
	persist.newUnitPersist = s.base.newUnitPersistence

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c,
		"Machines",

		"MachineUnits",

		"MachineUnits",
		"newUnitPersistence",
		"ListAll",
		"newUnitPersistence",
		"ListAll",
	)
}

func (s *envPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	persist := NewEnvPersistence(s.base)
	persist.newUnitPersist = s.base.newUnitPersistence

	_, err := persist.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

// TODO(ericsnow) Factor this out to a testing package.

func checkPayloads(c *gc.C, payloads []payload.FullPayloadInfo, expectedList ...payload.FullPayloadInfo) {
	remainder := make([]payload.FullPayloadInfo, len(payloads))
	copy(remainder, payloads)
	var noMatch []payload.FullPayloadInfo
	for _, expected := range expectedList {
		found := false
		for i, payload := range remainder {
			if reflect.DeepEqual(payload, expected) {
				remainder = append(remainder[:i], remainder[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			noMatch = append(noMatch, expected)
		}
	}

	ok1 := c.Check(noMatch, gc.HasLen, 0)
	ok2 := c.Check(remainder, gc.HasLen, 0)
	if !ok1 || !ok2 {
		c.Logf("<<<<<<<<\nexpected:")
		for _, payload := range expectedList {
			c.Logf("%#v", payload)
		}
		c.Logf("--------\ngot:")
		for _, payload := range payloads {
			c.Logf("%#v", payload)
		}
		c.Logf(">>>>>>>>")
	}
}

type stubEnvPersistenceBase struct {
	PersistenceBase
	stub         *testing.Stub
	machines     []string
	units        map[string]map[string]bool
	unitPersists map[string]*stubUnitPersistence
}

func (s *stubEnvPersistenceBase) setPayloads(payloads ...payload.FullPayloadInfo) {
	if s.unitPersists == nil && len(payloads) > 0 {
		s.unitPersists = make(map[string]*stubUnitPersistence)
	}

	for _, pl := range payloads {
		s.setUnits(pl.Machine, pl.Unit)

		unitPayloads := s.unitPersists[pl.Unit]
		if unitPayloads == nil {
			unitPayloads = &stubUnitPersistence{stub: s.stub}
			s.unitPersists[pl.Unit] = unitPayloads
		}

		unitPayloads.setPayloads(pl.Payload)
	}
}

func (s *stubEnvPersistenceBase) setUnits(machine string, units ...string) {
	if s.units == nil {
		s.units = make(map[string]map[string]bool)
	}
	if _, ok := s.units[machine]; !ok {
		s.machines = append(s.machines, machine)
		s.units[machine] = make(map[string]bool)
	}

	for _, unit := range units {
		s.units[machine][unit] = true
	}
}

func (s *stubEnvPersistenceBase) newUnitPersistence(base PersistenceBase, unit string) unitPersistence {
	s.stub.AddCall("newUnitPersistence", base, unit)
	s.stub.NextErr() // pop one off

	persist, ok := s.unitPersists[unit]
	if !ok {
		if s.unitPersists == nil {
			s.unitPersists = make(map[string]*stubUnitPersistence)
		}
		persist = &stubUnitPersistence{stub: s.stub}
		s.unitPersists[unit] = persist
	}
	return persist
}

func (s *stubEnvPersistenceBase) Machines() ([]string, error) {
	s.stub.AddCall("Machines")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var names []string
	for _, name := range s.machines {
		names = append(names, name)
	}
	return names, nil
}

func (s *stubEnvPersistenceBase) MachineUnits(machine string) ([]string, error) {
	s.stub.AddCall("MachineUnits", machine)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var units []string
	for unit := range s.units[machine] {
		units = append(units, unit)
	}
	return units, nil
}

type stubUnitPersistence struct {
	stub *testing.Stub

	payloads []payload.Payload
}

func (s *stubUnitPersistence) setPayloads(payloads ...payload.Payload) {
	s.payloads = append(s.payloads, payloads...)
}

func (s *stubUnitPersistence) ListAll() ([]payload.Payload, error) {
	s.stub.AddCall("ListAll")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.payloads, nil
}
