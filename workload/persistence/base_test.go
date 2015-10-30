// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"

	gitjujutesting "github.com/juju/testing"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
)

type BaseSuite struct {
	testing.BaseSuite

	Stub  *gitjujutesting.Stub
	State *fakeStatePersistence
	Unit  string
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Stub = &gitjujutesting.Stub{}
	s.State = &fakeStatePersistence{Stub: s.Stub}
	s.Unit = "a-unit/0"
}

type PayloadDoc payloadDoc

func (doc PayloadDoc) convert() *payloadDoc {
	return (*payloadDoc)(&doc)
}

func (s *BaseSuite) NewDoc(id string, pl workload.Payload) *payloadDoc {
	return &payloadDoc{
		DocID:  "payload#" + s.Unit + "#" + id,
		UnitID: s.Unit,

		Name:  pl.Name,
		Type:  pl.Type,
		RawID: pl.ID,
		State: pl.Status,
	}
}

func (s *BaseSuite) SetDoc(id string, pl workload.Payload) *payloadDoc {
	payloadDoc := s.NewDoc(id, pl)
	s.State.SetDocs(payloadDoc)
	return payloadDoc
}

func (s *BaseSuite) RemoveDoc(id string) {
	docID := "payload#" + s.Unit + "#" + id
	delete(s.State.docs, docID)
}

func (s *BaseSuite) NewPersistence() *Persistence {
	return NewPersistence(s.State, s.Unit)
}

func (s *BaseSuite) SetUnit(id string) {
	s.Unit = id
}

func (s *BaseSuite) NewPayloads(pType string, ids ...string) []workload.Payload {
	var payloads []workload.Payload
	for _, id := range ids {
		pl := s.NewPayload(pType, id)
		payloads = append(payloads, pl)
	}
	return payloads
}

func (s *BaseSuite) NewPayload(pType string, id string) workload.Payload {
	name, pluginID := workload.ParseID(id)
	if pluginID == "" {
		pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: pType,
		},
		ID:     pluginID,
		Status: "running",
		Unit:   s.Unit,
	}
}
