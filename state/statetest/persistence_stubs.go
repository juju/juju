// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statetest

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

type StubPersistence struct {
	*testing.Stub

	RunFunc func(jujutxn.TransactionSource) error

	ReturnAll interface{} // homegenous(?) list of doc struct (not pointers)
	ReturnOne interface{} // a doc struct (not a pointer)

	ReturnIncCharmModifiedVersionOps []txn.Op
}

func NewStubPersistence(stub *testing.Stub) *StubPersistence {
	s := &StubPersistence{
		Stub: stub,
	}
	s.RunFunc = s.run
	return s
}

func (s *StubPersistence) One(collName, id string, doc interface{}) error {
	s.AddCall("One", collName, id, doc)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if reflect.TypeOf(s.ReturnOne) == nil {
		return errors.NotFoundf("resource")
	}
	ptr := reflect.ValueOf(doc)
	newVal := reflect.ValueOf(s.ReturnOne)
	ptr.Elem().Set(newVal)
	return nil
}

func (s *StubPersistence) All(collName string, query, docs interface{}) error {
	s.AddCall("All", collName, query, docs)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	ptr := reflect.ValueOf(docs)
	if reflect.TypeOf(s.ReturnAll) == nil {
		ptr.Elem().SetLen(0)
	} else {
		newVal := reflect.ValueOf(s.ReturnAll)
		ptr.Elem().Set(newVal)
	}
	return nil
}

func (s *StubPersistence) Run(buildTxn jujutxn.TransactionSource) error {
	s.AddCall("Run", buildTxn)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if err := s.run(buildTxn); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// See github.com/juju/txn.transactionRunner.Run.
func (s *StubPersistence) run(buildTxn jujutxn.TransactionSource) error {
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

func (s *StubPersistence) RunTransaction(ops []txn.Op) error {
	s.AddCall("RunTransaction", ops)
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *StubPersistence) IncCharmModifiedVersionOps(serviceID string) []txn.Op {
	s.AddCall("IncCharmModifiedVersionOps", serviceID)
	// pop off an error so num errors == num calls, even though this call
	// doesn't actually use the error.
	s.NextErr()

	return s.ReturnIncCharmModifiedVersionOps
}
