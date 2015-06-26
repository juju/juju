// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&procsPersistenceSuite{})

type procsPersistenceSuite struct {
	baseProcessesSuite

	state *fakeStatePersistence
}

func (s *procsPersistenceSuite) SetUpTest(c *gc.C) {
	s.baseProcessesSuite.SetUpTest(c)

	s.state = &fakeStatePersistence{Stub: s.stub}
}

type processesPersistence interface {
	EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error)
	Insert(info process.Info) (bool, error)
	SetStatus(id string, status process.RawStatus) (bool, error)
	List(ids ...string) ([]process.Info, []string, error)
	Remove(id string) (bool, error)
}

func (s *procsPersistenceSuite) newPersistence() processesPersistence {
	return state.NewProcsPersistence(s.state, &s.charm, &s.unit)
}

func (s *procsPersistenceSuite) TestEnsureDefininitions(c *gc.C) {
	// TODO(ericsnow) finish!
}

func (s *procsPersistenceSuite) TestInsert(c *gc.C) {
	// TODO(ericsnow) finish!
}

func (s *procsPersistenceSuite) TestSetStatus(c *gc.C) {
	// TODO(ericsnow) finish!
}

func (s *procsPersistenceSuite) TestList(c *gc.C) {
	// TODO(ericsnow) finish!
}

func (s *procsPersistenceSuite) TestRemove(c *gc.C) {
	// TODO(ericsnow) finish!
}

type fakeStatePersistence struct {
	*gitjujutesting.Stub

	docs map[string]interface{}
	ops  [][]txn.Op
}

func (sp fakeStatePersistence) One(collName, id string, doc interface{}) error {
	sp.AddCall("One", collName, id, doc)
	if err := sp.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if len(sp.docs) == 0 {
		return errors.NotFoundf(id)
	}
	found, ok := sp.docs[id]
	if !ok {
		return errors.NotFoundf(id)
	}
	state.ProcsUpdateDoc(doc, found)

	return nil
}

func (sp fakeStatePersistence) All(collName string, ids []string, docs interface{}) error {
	sp.AddCall("All", collName, ids, docs)
	if err := sp.NextErr(); err != nil {
		return errors.Trace(err)
	}

	var found []interface{}
	for _, id := range ids {
		doc, ok := sp.docs[id]
		if !ok {
			continue
		}
		found = append(found, doc)
	}
	actual := docs.(*[]interface{})
	*actual = found
	return nil
}

func (sp fakeStatePersistence) Run(transactions jujutxn.TransactionSource) error {
	sp.AddCall("Run", transactions)

	// See transactionRunner.Run in github.com/juju/txn.
	for i := 0; ; i++ {
		const nrRetries = 3
		if i >= nrRetries {
			return jujutxn.ErrExcessiveContention
		}

		// Get the ops.
		ops, err := transactions(i)
		if err == jujutxn.ErrTransientFailure {
			continue
		}
		if err == jujutxn.ErrNoOperations {
			break
		}
		if err != nil {
			return err
		}

		// "run" the ops.
		sp.ops = append(sp.ops, ops)
		if err := sp.NextErr(); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return err
		}
	}
	return nil
}
