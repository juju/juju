// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
)

type PayloadPersistenceFixture struct {
	Stub    *testing.Stub
	DB      *StubPayloadsPersistenceBase
	Queries payloadsQueries
	StateID string
}

func NewPayloadPersistenceFixture() *PayloadPersistenceFixture {
	stub := &testing.Stub{}
	db := &StubPayloadsPersistenceBase{Stub: stub}
	return &PayloadPersistenceFixture{
		Stub:    stub,
		DB:      db,
		Queries: payloadsQueries{db},
		StateID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	}
}

func (f PayloadPersistenceFixture) NewPersistence() *PayloadsPersistence {
	return NewPayloadsPersistence(f.DB)
}

func (f PayloadPersistenceFixture) NewPayload(machine, unit, pType string, id string) payload.FullPayloadInfo {
	name, pluginID := payload.ParseID(id)
	if pluginID == "" {
		pluginID = fmt.Sprintf("%s-%s", name, utils.MustNewUUID())
	}

	return payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: pType,
			},
			ID:     pluginID,
			Status: "running",
			Unit:   unit,
		},
		Machine: machine,
	}
}

func (f PayloadPersistenceFixture) NewPayloads(machine, unit, pType string, ids ...string) []payload.FullPayloadInfo {
	var payloads []payload.FullPayloadInfo
	for _, id := range ids {
		pl := f.NewPayload(machine, unit, pType, id)
		payloads = append(payloads, pl)
	}
	return payloads
}

func (f PayloadPersistenceFixture) SetDocs(payloads ...payload.FullPayloadInfo) {
	f.DB.SetDocs(payloads...)
}

func (f PayloadPersistenceFixture) CheckPayloads(c *gc.C, payloads []payload.FullPayloadInfo, expectedList ...payload.FullPayloadInfo) {
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

type StubPayloadsPersistenceBase struct {
	*testing.Stub

	ReturnAll []*payloadDoc
}

func (s *StubPayloadsPersistenceBase) AddDoc(pl payload.FullPayloadInfo) {
	doc := newPayloadDoc(pl)
	s.ReturnAll = append(s.ReturnAll, doc)
}

func (s *StubPayloadsPersistenceBase) SetDocs(payloads ...payload.FullPayloadInfo) {
	docs := make([]*payloadDoc, len(payloads))
	for i, pl := range payloads {
		docs[i] = newPayloadDoc(pl)
	}
	s.ReturnAll = docs
}

func (s *StubPayloadsPersistenceBase) All(collName string, query, docs interface{}) error {
	s.AddCall("All", collName, query, docs)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	actual := docs.(*[]payloadDoc)
	var copied []payloadDoc
	for _, doc := range s.ReturnAll {
		copied = append(copied, *doc)
	}
	*actual = copied
	return nil
}

func (s *StubPayloadsPersistenceBase) Run(transactions jujutxn.TransactionSource) error {
	s.AddCall("Run", transactions)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
