// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
)

type StubPersistenceBase struct {
	*testing.Stub

	ReturnAll []*payloadDoc
}

func (s *StubPersistenceBase) AddDoc(stID string, pl payload.FullPayloadInfo) {
	doc := newPayloadDoc(stID, pl)
	s.ReturnAll = append(s.ReturnAll, doc)
}

func (s *StubPersistenceBase) SetDoc(stID string, pl payload.FullPayloadInfo) {
	doc := newPayloadDoc(stID, pl)
	s.ReturnAll = []*payloadDoc{doc}
}

func (s *StubPersistenceBase) SetDocs(payloads ...payload.FullPayloadInfo) {
	var docs []*payloadDoc
	for i, pl := range payloads {
		stID := fmt.Sprint(i)
		doc := newPayloadDoc(stID, pl)
		docs = append(docs, doc)
	}
	s.ReturnAll = docs
}

func (s *StubPersistenceBase) All(collName string, query, docs interface{}) error {
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

func (s *StubPersistenceBase) Run(transactions jujutxn.TransactionSource) error {
	s.AddCall("Run", transactions)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func NewPayload(machine, unit, pType string, id string) payload.FullPayloadInfo {
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
