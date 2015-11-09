// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type fakeStatePersistence struct {
	*gitjujutesting.Stub

	docs map[string]*payloadDoc
	ops  [][]txn.Op
}

func (sp *fakeStatePersistence) SetDocs(docs ...*payloadDoc) {
	if sp.docs == nil {
		sp.docs = make(map[string]*payloadDoc)
	}
	for _, doc := range docs {
		sp.docs[doc.DocID] = doc
	}
}

func (sp fakeStatePersistence) CheckOps(c *gc.C, expected [][]txn.Op) {
	if len(sp.ops) != len(expected) {
		c.Check(sp.ops, jc.DeepEquals, expected)
		return
	}

	for i, ops := range sp.ops {
		c.Logf(" -- txn attempt %d --\n", i)
		expectedRun := expected[i]
		if len(ops) != len(expectedRun) {
			c.Check(ops, jc.DeepEquals, expectedRun)
			continue
		}
		for j, op := range ops {
			c.Logf(" <op %d>\n", j)
			expectedOp := expectedRun[j]
			if expectedOp.Insert != nil {
				if doc, ok := expectedOp.Insert.(*PayloadDoc); ok {
					expectedOp.Insert = doc.convert()
				}
			} else if expectedOp.Update != nil {
				if doc, ok := expectedOp.Update.(*PayloadDoc); ok {
					expectedOp.Update = doc.convert()
				}
			}
			c.Check(op, jc.DeepEquals, expectedOp)
		}
	}
}

func (sp fakeStatePersistence) CheckNoOps(c *gc.C) {
	c.Check(sp.ops, gc.HasLen, 0)
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
	actual := doc.(*payloadDoc)
	*actual = *found
	return nil
}

func (sp fakeStatePersistence) All(collName string, query, docs interface{}) error {
	sp.AddCall("All", collName, query, docs)
	if err := sp.NextErr(); err != nil {
		return errors.Trace(err)
	}

	var ids []string
	elems := query.(bson.D)
	if len(elems) < 1 {
		err := errors.Errorf("bad query %v", query)
		panic(err)
	}
	switch elems[0].Name {
	case "_id":
		if len(elems) != 1 {
			err := errors.Errorf("bad query %v", query)
			panic(err)
		}
		elems = elems[0].Value.(bson.D)
		if len(elems) != 1 || elems[0].Name != "$in" {
			err := errors.Errorf("bad query %v", query)
			panic(err)
		}
		ids = elems[0].Value.([]string)
	case "unitid":
		for id := range sp.docs {
			ids = append(ids, id)
		}
	default:
		panic(query)
	}

	var found []payloadDoc
	for _, id := range ids {
		doc, ok := sp.docs[id]
		if !ok {
			continue
		}
		found = append(found, *doc)
	}
	actual := docs.(*[]payloadDoc)
	*actual = found
	return nil
}

func (sp *fakeStatePersistence) Run(transactions jujutxn.TransactionSource) error {
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
