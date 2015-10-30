// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/payload"
)

type baseWorkloadsSuite struct {
	testing.BaseSuite

	stub    *gitjujutesting.Stub
	persist *fakeWorkloadsPersistence
}

func (s *baseWorkloadsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.persist = &fakeWorkloadsPersistence{Stub: s.stub}
}

func (s *baseWorkloadsSuite) newPayload(pType string, id string) payload.Payload {
	name, rawID := payload.ParseID(id)
	if rawID == "" {
		rawID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		Status: payload.StateRunning,
		ID:     rawID,
		Unit:   "a-service/0",
	}
}

type fakeWorkloadsPersistence struct {
	*gitjujutesting.Stub
	payloads map[string]*payload.Payload
}

func (s *fakeWorkloadsPersistence) checkPayload(c *gc.C, id string, expected payload.Payload) {
	pl, ok := s.payloads[id]
	if !ok {
		c.Errorf("payload %q not found", id)
	} else {
		c.Check(pl, jc.DeepEquals, &expected)
	}
}

func (s *fakeWorkloadsPersistence) setPayload(id string, pl *payload.Payload) {
	if s.payloads == nil {
		s.payloads = make(map[string]*payload.Payload)
	}
	s.payloads[id] = pl
}

func (s *fakeWorkloadsPersistence) Track(id string, pl payload.Payload) (bool, error) {
	s.AddCall("Track", id, pl)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.payloads[id]; ok {
		return false, nil
	}
	s.setPayload(id, &pl)
	return true, nil
}

func (s *fakeWorkloadsPersistence) SetStatus(id, status string) (bool, error) {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	pl, ok := s.payloads[id]
	if !ok {
		return false, nil
	}
	pl.Status = status
	return true, nil
}

func (s *fakeWorkloadsPersistence) List(ids ...string) ([]payload.Payload, []string, error) {
	s.AddCall("List", ids)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var payloads []payload.Payload
	var missing []string
	for _, id := range ids {
		if pl, ok := s.payloads[id]; !ok {
			missing = append(missing, id)
		} else {
			payloads = append(payloads, *pl)
		}
	}
	return payloads, missing, nil
}

func (s *fakeWorkloadsPersistence) ListAll() ([]payload.Payload, error) {
	s.AddCall("ListAll")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []payload.Payload
	for _, pl := range s.payloads {
		payloads = append(payloads, *pl)
	}
	return payloads, nil
}

func (s *fakeWorkloadsPersistence) LookUp(name, rawID string) (string, error) {
	s.AddCall("LookUp", name, rawID)
	if err := s.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	for id, pl := range s.payloads {
		if pl.Name == name && pl.ID == rawID {
			return id, nil
		}
	}
	return "", errors.NotFoundf("doc ID")
}

func (s *fakeWorkloadsPersistence) Untrack(id string) (bool, error) {
	s.AddCall("Untrack", id)
	if err := s.NextErr(); err != nil {
		return false, errors.Trace(err)
	}

	if _, ok := s.payloads[id]; !ok {
		return false, nil
	}
	delete(s.payloads, id)
	return true, nil
}
