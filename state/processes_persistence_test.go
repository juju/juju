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

func (s *procsPersistenceSuite) TestEnsureDefininitionsCharmAndUnit(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA")
	s.setUnit("a-unit/0")

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procA",
				UnitID: "a-unit/0",
				Name:   "procA",
				Type:   "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsCharmOnly(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA")
	s.setUnit("")

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID: "c#local:series/dummy-1#procA",
				Name:  "procA",
				Type:  "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMultiple(c *gc.C) {
	definitions := s.newDefinitions("docker", "procA", "procB")
	s.setUnit("")

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID: "c#local:series/dummy-1#procA",
				Name:  "procA",
				Type:  "docker",
			},
		}, {
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procB",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID: "c#local:series/dummy-1#procB",
				Name:  "procB",
				Type:  "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsEmpty(c *gc.C) {
	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.stub.CheckCallNames(c)
	c.Check(s.state.ops, gc.HasLen, 0)
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	definitions := s.newDefinitions("docker", "procA")
	s.setUnit("")

	pp := s.newPersistence()
	_, _, err := pp.EnsureDefinitions(definitions...)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsFound(c *gc.C) {
	s.stub.SetErrors(txn.ErrAborted)
	definitions := s.newDefinitions("docker", "procA")
	s.setUnit("")
	expected := &state.ProcessDefinitionDoc{
		DocID: "c#local:series/dummy-1#procA",
		Name:  "procA",
		Type:  "docker",
	}
	s.state.setDocs(expected)

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"c#local:series/dummy-1#procA",
	})
	c.Check(mismatched, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "Run", "All")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: expected,
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMismatched(c *gc.C) {
	s.stub.SetErrors(txn.ErrAborted)
	definitions := s.newDefinitions("kvm", "procA")
	s.setUnit("")
	doc := &state.ProcessDefinitionDoc{
		DocID: "c#local:series/dummy-1#procA",
		Name:  "procA",
		Type:  "docker",
	}
	s.state.setDocs(doc)

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"c#local:series/dummy-1#procA",
	})
	c.Check(mismatched, jc.DeepEquals, []string{
		"c#local:series/dummy-1#procA",
	})
	s.stub.CheckCallNames(c, "Run", "All")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID: "c#local:series/dummy-1#procA",
				Name:  "procA",
				Type:  "kvm",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMixed(c *gc.C) {
	s.stub.SetErrors(txn.ErrAborted)
	definitions := s.newDefinitions("kvm", "procA")
	definitions = append(definitions, s.newDefinitions("docker", "procB", "procC")...)
	s.setUnit("a-unit/0")
	doc := &state.ProcessDefinitionDoc{
		DocID:  "c#local:series/dummy-1#procA",
		Name:   "procA",
		UnitID: "a-unit/0",
		Type:   "docker",
	}
	expected := &state.ProcessDefinitionDoc{
		DocID:  "c#local:series/dummy-1#procB",
		Name:   "procB",
		UnitID: "a-unit/0",
		Type:   "docker",
	}
	s.state.setDocs(doc, expected)

	pp := s.newPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"c#local:series/dummy-1#procA",
		"c#local:series/dummy-1#procB",
	})
	c.Check(mismatched, jc.DeepEquals, []string{
		"c#local:series/dummy-1#procA",
	})
	s.stub.CheckCallNames(c, "Run", "All")
	s.state.checkOps(c, [][]txn.Op{{
		// first attempt
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procA",
				Name:   "procA",
				UnitID: "a-unit/0",
				Type:   "kvm",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procB",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procB",
				Name:   "procB",
				UnitID: "a-unit/0",
				Type:   "docker",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procC",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procC",
				Name:   "procC",
				UnitID: "a-unit/0",
				Type:   "docker",
			},
		},
	}, {
		// second attempt
		{
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procC",
			Assert: txn.DocMissing,
			Insert: &state.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procC",
				Name:   "procC",
				UnitID: "a-unit/0",
				Type:   "docker",
			},
		},
	}})
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

func (sp *fakeStatePersistence) setDocs(docs ...interface{}) {
	if sp.docs == nil {
		sp.docs = make(map[string]interface{})
	}
	for _, doc := range docs {
		var id string
		switch doc := doc.(type) {
		case *state.ProcessDefinitionDoc:
			id = doc.DocID
		case *state.ProcessLaunchDoc:
			id = doc.DocID
		case *state.ProcessDoc:
			id = doc.DocID
		default:
			panic(doc)
		}
		if id == "" {
			panic(doc)
		}
		sp.docs[id] = doc
	}
}

func (sp fakeStatePersistence) checkOps(c *gc.C, expected [][]txn.Op) {
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
			c.Check(op, jc.DeepEquals, expectedRun[j])
		}
	}
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

	switch doc := doc.(type) {
	case *state.ProcessDefinitionDoc:
		expected := found.(*state.ProcessDefinitionDoc)
		*doc = *expected
	case *state.ProcessLaunchDoc:
		expected := found.(*state.ProcessLaunchDoc)
		*doc = *expected
	case *state.ProcessDoc:
		expected := found.(*state.ProcessDoc)
		*doc = *expected
	default:
		panic(doc)
	}

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
	switch docs := docs.(type) {
	case *[]state.ProcessDefinitionDoc:
		var found []state.ProcessDefinitionDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*state.ProcessDefinitionDoc))
		}
		*docs = found
	case *[]state.ProcessLaunchDoc:
		var found []state.ProcessLaunchDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*state.ProcessLaunchDoc))
		}
		*docs = found
	case *[]state.ProcessDoc:
		var found []state.ProcessDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*state.ProcessDoc))
		}
		*docs = found
	default:
		panic(docs)
	}
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
