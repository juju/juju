// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

type stubStatePersistence struct {
	stub *testing.Stub

	docs      []resourceDoc
	ReturnOne resourceDoc
}

func (s stubStatePersistence) One(collName, id string, doc interface{}) error {
	s.stub.AddCall("One", collName, id, doc)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	actual := doc.(*resourceDoc)
	*actual = s.ReturnOne
	return nil
}

func (s stubStatePersistence) All(collName string, query, docs interface{}) error {
	s.stub.AddCall("All", collName, query, docs)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	actual := docs.(*[]resourceDoc)
	*actual = s.docs
	return nil
}

func (s stubStatePersistence) Run(buildTxn jujutxn.TransactionSource) error {
	s.stub.AddCall("Run", buildTxn)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if err := s.run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// See github.com/juju/txn.transactionRunner.Run.
func (s stubStatePersistence) run(buildTxn jujutxn.TransactionSource) error {
	for i := 0; ; i++ {
		ops, err := buildTxn(i)
		if errors.Cause(err) == jujutxn.ErrTransientFailure {
			continue
		}
		if errors.Cause(err) == jujutxn.ErrNoOperations {
			return nil
		}
		if err != nil {
			return err
		}

		err = s.RunTransaction(ops)
		if errors.Cause(err) == txn.ErrAborted {
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (s stubStatePersistence) RunTransaction(ops []txn.Op) error {
	s.stub.AddCall("RunTransaction", ops)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
