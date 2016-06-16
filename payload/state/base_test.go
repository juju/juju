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
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/testing"
)

type basePayloadsSuite struct {
	testing.BaseSuite

	stub    *gitjujutesting.Stub
	persist *fakePayloadsPersistence
}

func (s *basePayloadsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.persist = &fakePayloadsPersistence{Stub: s.stub}
}

func (s *basePayloadsSuite) newPayload(pType string, id string) payload.FullPayloadInfo {
	name, rawID := payload.ParseID(id)
	if rawID == "" {
		rawID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: pType,
			},
			Status: payload.StateRunning,
			ID:     rawID,
			Unit:   "a-application/0",
		},
		Machine: "0",
	}
}

type fakePayloadsPersistence struct {
	*gitjujutesting.Stub
	payloads map[string]*payload.FullPayloadInfo
}

func (s *fakePayloadsPersistence) checkPayload(c *gc.C, id string, expected payload.FullPayloadInfo) {
	pl, ok := s.payloads[id]
	if !ok {
		c.Errorf("payload %q not found", id)
	} else {
		c.Check(pl, jc.DeepEquals, &expected)
	}
}

func (s *fakePayloadsPersistence) setPayload(pl *payload.FullPayloadInfo) {
	if s.payloads == nil {
		s.payloads = make(map[string]*payload.FullPayloadInfo)
	}
	s.payloads[pl.Name] = pl
}

func (s *fakePayloadsPersistence) Track(pl payload.FullPayloadInfo) error {
	s.AddCall("Track", pl)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if _, ok := s.payloads[pl.Name]; ok {
		return payload.ErrAlreadyExists
	}
	s.setPayload(&pl)
	return nil
}

func (s *fakePayloadsPersistence) SetStatus(id, status string) error {
	s.AddCall("SetStatus", id, status)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	pl, ok := s.payloads[id]
	if !ok {
		return payload.ErrNotFound
	}
	pl.Status = status
	return nil
}

func (s *fakePayloadsPersistence) List(ids ...string) ([]payload.FullPayloadInfo, []string, error) {
	s.AddCall("List", ids)
	if err := s.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	var payloads []payload.FullPayloadInfo
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

func (s *fakePayloadsPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	s.AddCall("ListAll")
	if err := s.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []payload.FullPayloadInfo
	for _, pl := range s.payloads {
		payloads = append(payloads, *pl)
	}
	return payloads, nil
}

func (s *fakePayloadsPersistence) Untrack(id string) error {
	s.AddCall("Untrack", id)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if _, ok := s.payloads[id]; !ok {
		return payload.ErrNotFound
	}
	delete(s.payloads, id)
	return nil
}
