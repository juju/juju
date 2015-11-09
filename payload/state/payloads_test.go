// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/state"
)

var _ = gc.Suite(&envPayloadsSuite{})

type envPayloadsSuite struct {
	basePayloadsSuite

	persists *stubPayloadsPersistence
}

func (s *envPayloadsSuite) SetUpTest(c *gc.C) {
	s.basePayloadsSuite.SetUpTest(c)

	s.persists = &stubPayloadsPersistence{stub: s.stub}
}

func (s *envPayloadsSuite) newPayload(name string) payload.FullPayloadInfo {
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

func (s *envPayloadsSuite) TestListAllOkay(c *gc.C) {
	id1 := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id2 := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	p1 := s.newPayload("spam")
	p2 := s.newPayload("eggs")
	s.persists.setPayload(id1, p1)
	s.persists.setPayload(id2, p2)

	ps := state.EnvPayloads{
		Persist: s.persists,
	}
	payloads, err := ps.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll", "ListAll")
	checkPayloads(c, payloads, []payload.FullPayloadInfo{
		p1,
		p2,
	})
}

func (s *envPayloadsSuite) TestListAllMulti(c *gc.C) {
	id1 := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id2 := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	id3 := "f47ac10b-58cc-4372-a567-0e02b2c3d481"
	id4 := "f47ac10b-58cc-4372-a567-0e02b2c3d482"
	p1 := s.newPayload("spam")
	p2 := s.newPayload("eggs")
	p2.Unit = "a-service/1"
	p3 := s.newPayload("ham")
	p3.Unit = "a-service/2"
	p3.Machine = "2"
	p4 := s.newPayload("spamspamspam")
	p4.Unit = "a-service/1"
	s.persists.setPayload(id1, p1)
	s.persists.setPayload(id2, p2)
	s.persists.setPayload(id3, p3)
	s.persists.setPayload(id4, p4)

	ps := state.EnvPayloads{
		Persist: s.persists,
	}
	payloads, err := ps.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll", "ListAll", "ListAll", "ListAll")
	checkPayloads(c, payloads, []payload.FullPayloadInfo{
		p1,
		p2,
		p3,
		p4,
	})
}

func (s *envPayloadsSuite) TestListAllFailed(c *gc.C) {
	id1 := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id2 := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	p1 := s.newPayload("spam")
	p2 := s.newPayload("eggs")
	s.persists.setPayload(id1, p1)
	s.persists.setPayload(id2, p2)

	ps := state.EnvPayloads{
		Persist: s.persists,
	}
	_, err := ps.ListAll()

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func checkPayloads(c *gc.C, payloads, expectedList []payload.FullPayloadInfo) {
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

type stubPayloadsPersistence struct {
	stub *testing.Stub

	persists map[string]map[string]*fakePayloadsPersistence
}

func (s *stubPayloadsPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	s.stub.AddCall("ListAll")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var fullPayloads []payload.FullPayloadInfo
	for machine, units := range s.persists {
		for unit, unitPayloads := range units {
			payloads, err := unitPayloads.ListAll()
			if err != nil {
				return nil, errors.Trace(err)
			}

			for _, pl := range payloads {
				if pl.Unit == "" {
					pl.Unit = unit
				}
				fullPayload := payload.FullPayloadInfo{
					Payload: pl,
					Machine: machine,
				}
				fullPayloads = append(fullPayloads, fullPayload)
			}
		}
	}
	return fullPayloads, nil
}

func (s *stubPayloadsPersistence) setPayload(id string, pl payload.FullPayloadInfo) {
	if s.persists == nil {
		s.persists = make(map[string]map[string]*fakePayloadsPersistence)
	}

	units := s.persists[pl.Machine]
	if units == nil {
		units = make(map[string]*fakePayloadsPersistence)
		s.persists[pl.Machine] = units
	}
	unitPayloads := units[pl.Unit]
	if unitPayloads == nil {
		unitPayloads = &fakePayloadsPersistence{Stub: s.stub}
		units[pl.Unit] = unitPayloads
	}

	unitPayloads.setPayload(id, &pl.Payload)
}
