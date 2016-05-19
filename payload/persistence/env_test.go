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
		fakeStatePersistence: s.State,
		stub:                 s.Stub,
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

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, p1, p2)
	s.Stub.CheckCallNames(c, "All")
}

func (s *envPersistenceSuite) TestListAllEmpty(c *gc.C) {
	s.base.setUnits("0")
	s.base.setUnits("1", "a-service/0", "a-service/1")
	persist := NewEnvPersistence(s.base)

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "All")
}

func (s *envPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	persist := NewEnvPersistence(s.base)

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
	*fakeStatePersistence
	stub         *testing.Stub
	unitMachines map[string]string
}

func (s *stubEnvPersistenceBase) setPayloads(payloads ...payload.FullPayloadInfo) {
	for _, pl := range payloads {
		s.setUnits(pl.Machine, pl.Unit)

		doc := newPayloadDoc("0", pl)
		s.SetDocs(doc)
	}
}

func (s *stubEnvPersistenceBase) setUnits(machine string, units ...string) {
	if s.unitMachines == nil {
		s.unitMachines = make(map[string]string)
	}
	for _, unit := range units {
		s.unitMachines[unit] = machine
	}
}
