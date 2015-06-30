// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"strings"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/persistence"
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
	SetStatus(id string, status process.Status) (bool, error)
	List(ids ...string) ([]process.Info, []string, error)
	ListAll() ([]process.Info, error)
	Remove(id string) (bool, error)
}

func (s *procsPersistenceSuite) newPersistence() processesPersistence {
	return persistence.NewPersistence(s.state, &s.charm, &s.unit)
}

type processInfoDoc struct {
	definition *persistence.ProcessDefinitionDoc
	launch     *persistence.ProcessLaunchDoc
	proc       *persistence.ProcessDoc
}

func (s *procsPersistenceSuite) setDocs(procs ...process.Info) []processInfoDoc {
	var results []processInfoDoc
	var docs []interface{}
	for _, proc := range procs {
		doc := processInfoDoc{}

		doc.definition = &persistence.ProcessDefinitionDoc{
			DocID:  "c#" + s.charm.Id() + "#" + proc.Name,
			Name:   proc.Name,
			Type:   proc.Type,
			UnitID: s.unit.Id(),
		}
		docs = append(docs, doc.definition)

		if proc.Details.ID != "" {
			doc.launch = &persistence.ProcessLaunchDoc{
				DocID:     "u#" + s.unit.Id() + "#charm#" + proc.ID() + "#launch",
				PluginID:  proc.Details.ID,
				RawStatus: proc.Details.Status.Label,
			}
			doc.proc = &persistence.ProcessDoc{
				DocID:        "u#" + s.unit.Id() + "#charm#" + proc.ID(),
				Life:         0,
				PluginStatus: proc.Details.Status.Label,
			}
			docs = append(docs, doc.launch, doc.proc)
		}

		results = append(results, doc)
	}
	s.state.setDocs(docs...)
	return results
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
			Insert: &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
				DocID: "c#local:series/dummy-1#procA",
				Name:  "procA",
				Type:  "docker",
			},
		}, {
			C:      "workloadprocesses",
			Id:     "c#local:series/dummy-1#procB",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDefinitionDoc{
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
	expected := &persistence.ProcessDefinitionDoc{
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
	doc := &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
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
	doc := &persistence.ProcessDefinitionDoc{
		DocID:  "c#local:series/dummy-1#procA",
		Name:   "procA",
		UnitID: "a-unit/0",
		Type:   "docker",
	}
	expected := &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
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
			Insert: &persistence.ProcessDefinitionDoc{
				DocID:  "c#local:series/dummy-1#procC",
				Name:   "procC",
				UnitID: "a-unit/0",
				Type:   "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]

	pp := s.newPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessLaunchDoc{
				DocID:     "u#a-unit/0#charm#procA/procA-xyz#launch",
				PluginID:  "procA-xyz",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "u#a-unit/0#charm#procA/procA-xyz",
				Life:         0,
				PluginStatus: "running",
			},
		},
		// TODO(ericsnow) This op will be there once we add definitions.
		//{
		//	C:      "workloadprocesses",
		//	Id:     "c#local:series/dummy-1#procA",
		//	Assert: txn.DocMissing,
		//	Insert: &persistence.ProcessDefinitionDoc{
		//		DocID: "c#local:series/dummy-1#procA",
		//		Name:  "procA",
		//		Type:  "docker",
		//	},
		//},
	}})
}

func (s *procsPersistenceSuite) TestInsertDefinitionExists(c *gc.C) {
	expected := &persistence.ProcessDefinitionDoc{
		DocID: "c#local:series/dummy-1#procA",
		Name:  "procA",
		Type:  "docker",
	}
	s.state.setDocs(expected)
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]

	pp := s.newPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessLaunchDoc{
				DocID:     "u#a-unit/0#charm#procA/procA-xyz#launch",
				PluginID:  "procA-xyz",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "u#a-unit/0#charm#procA/procA-xyz",
				Life:         0,
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertDefinitionMismatch(c *gc.C) {
	expected := &persistence.ProcessDefinitionDoc{
		DocID: "c#local:series/dummy-1#procA",
		Name:  "procA",
		Type:  "docker",
	}
	s.state.setDocs(expected)
	proc := s.newProcesses("kvm", "procA/procA-xyz")[0]

	pp := s.newPersistence()
	okay, err := pp.Insert(proc)
	// TODO(ericsnow) Should this fail instead?
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessLaunchDoc{
				DocID:     "u#a-unit/0#charm#procA/procA-xyz#launch",
				PluginID:  "procA-xyz",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "u#a-unit/0#charm#procA/procA-xyz",
				Life:         0,
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertAlreadyExists(c *gc.C) {
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]
	s.setDocs(proc)
	s.stub.SetErrors(txn.ErrAborted)

	pp := s.newPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessLaunchDoc{
				DocID:     "u#a-unit/0#charm#procA/procA-xyz#launch",
				PluginID:  "procA-xyz",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "u#a-unit/0#charm#procA/procA-xyz",
				Life:         0,
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	proc := s.newProcesses("docker", "procA")[0]

	pp := s.newPersistence()
	_, err := pp.Insert(proc)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]
	s.setDocs(proc)
	newStatus := process.Status{Label: "still running"}

	pp := s.newPersistence()
	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocExists,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: persistence.IsAliveDoc,
			Update: bson.D{
				{"$set", bson.D{{"pluginstatus", "still running"}}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	s.stub.SetErrors(txn.ErrAborted)
	newStatus := process.Status{Label: "still running"}

	pp := s.newPersistence()
	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.stub.CheckCallNames(c, "Run", "One")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocExists,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: persistence.IsAliveDoc,
			Update: bson.D{
				{"$set", bson.D{{"pluginstatus", "still running"}}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusDying(c *gc.C) {
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]
	docs := s.setDocs(proc)
	docs[0].proc.Life = persistence.Dying
	s.stub.SetErrors(txn.ErrAborted)
	newStatus := process.Status{Label: "still running"}

	pp := s.newPersistence()
	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run", "One")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: txn.DocExists,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/procA-xyz",
			Assert: persistence.IsAliveDoc,
			Update: bson.D{
				{"$set", bson.D{{"pluginstatus", "still running"}}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	proc := s.newProcesses("docker", "procA/procA-xyz")[0]
	s.setDocs(proc)
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	pp := s.newPersistence()
	_, err := pp.SetStatus("procA/procA-xyz", process.Status{Label: "still running"})

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestListOkay(c *gc.C) {
	existing := s.newProcesses("docker", "procA/xyz", "procB/abc")
	s.setDocs(existing...)

	pp := s.newPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	c.Check(procs, jc.DeepEquals, []process.Info{existing[0]})
	c.Check(missing, gc.HasLen, 0)
}

func (s *procsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	existing := s.newProcesses("docker", "procA/xyz", "procB/abc")
	s.setDocs(existing...)

	pp := s.newPersistence()
	procs, missing, err := pp.List("procB/abc", "procC/123")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	c.Check(procs, jc.DeepEquals, []process.Info{existing[1]})
	c.Check(missing, jc.DeepEquals, []string{"procC/123"})
}

func (s *procsPersistenceSuite) TestListEmpty(c *gc.C) {
	pp := s.newPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	c.Check(procs, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{"procA/xyz"})
}

func (s *procsPersistenceSuite) TestListInconsistent(c *gc.C) {
	existing := s.newProcesses("docker", "procA/xyz", "procB/abc")
	s.setDocs(existing...)
	delete(s.state.docs, "u#a-unit/0#charm#procA/xyz#launch")

	pp := s.newPersistence()
	_, _, err := pp.List("procA/xyz")

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestListFailure(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	pp := s.newPersistence()
	_, _, err := pp.List()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestListAllOkay(c *gc.C) {
	existing := s.newProcesses("docker", "procA/xyz", "procB/abc")
	s.setDocs(existing...)

	pp := s.newPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	c.Check(procs, jc.DeepEquals, existing)
}

func (s *procsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	pp := s.newPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	c.Check(procs, gc.HasLen, 0)
}

func (s *procsPersistenceSuite) TestListAllIncludeCharmDefined(c *gc.C) {
	s.state.setDocs(&persistence.ProcessDefinitionDoc{
		DocID: "c#local:series/dummy-1#procA",
		Name:  "procA",
		Type:  "docker",
	})
	existing := s.newProcesses("docker", "procB/abc", "procC/xyz")
	s.setDocs(existing...)

	pp := s.newPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "All", "All", "All")
	c.Check(s.state.ops, gc.HasLen, 0)
	existing = append(existing, process.Info{
		Process: charm.Process{
			Name: "procA",
			Type: "docker",
		},
	})
	c.Check(procs, jc.DeepEquals, existing)
}

func (s *procsPersistenceSuite) TestListAllInconsistent(c *gc.C) {
	existing := s.newProcesses("docker", "procA/xyz", "procB/abc")
	s.setDocs(existing...)
	delete(s.state.docs, "u#a-unit/0#charm#procA/xyz#launch")

	pp := s.newPersistence()
	_, err := pp.ListAll()

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	pp := s.newPersistence()
	_, err := pp.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestRemoveOkay(c *gc.C) {
	proc := s.newProcesses("docker", "procA/xyz")[0]
	s.setDocs(proc)

	pp := s.newPersistence()
	found, err := pp.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz#launch",
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: persistence.IsAliveDoc,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *procsPersistenceSuite) TestRemoveMissing(c *gc.C) {
	s.stub.SetErrors(txn.ErrAborted)

	pp := s.newPersistence()
	found, err := pp.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsFalse)
	s.stub.CheckCallNames(c, "Run", "One", "One", "One")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz#launch",
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: persistence.IsAliveDoc,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *procsPersistenceSuite) TestRemoveDying(c *gc.C) {
	proc := s.newProcesses("docker", "procA/xyz")[0]
	docs := s.setDocs(proc)
	docs[0].proc.Life = persistence.Dying

	pp := s.newPersistence()
	found, err := pp.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.stub.CheckCallNames(c, "Run")
	s.state.checkOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz#launch",
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: persistence.IsAliveDoc,
		}, {
			C:      "workloadprocesses",
			Id:     "u#a-unit/0#charm#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *procsPersistenceSuite) TestRemoveInconsistent(c *gc.C) {
	proc := s.newProcesses("docker", "procA/xyz")[0]
	s.setDocs(proc)
	delete(s.state.docs, "u#a-unit/0#charm#procA/xyz#launch")
	s.stub.SetErrors(txn.ErrAborted)

	pp := s.newPersistence()
	_, err := pp.Remove("procA/xyz")

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestRemoveFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	pp := s.newPersistence()
	_, err := pp.Remove("procA/xyz")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

type fakeStatePersistence struct {
	*gitjujutesting.Stub

	docs           map[string]interface{}
	definitionDocs []string
	launchDocs     []string
	procDocs       []string
	ops            [][]txn.Op
}

func (sp *fakeStatePersistence) setDocs(docs ...interface{}) {
	if sp.docs == nil {
		sp.docs = make(map[string]interface{})
	}
	for _, doc := range docs {
		var id string
		switch doc := doc.(type) {
		case *persistence.ProcessDefinitionDoc:
			id = doc.DocID
			sp.definitionDocs = append(sp.definitionDocs, id)
		case *persistence.ProcessLaunchDoc:
			id = doc.DocID
			sp.launchDocs = append(sp.launchDocs, id)
		case *persistence.ProcessDoc:
			id = doc.DocID
			sp.procDocs = append(sp.procDocs, id)
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
	case *persistence.ProcessDefinitionDoc:
		expected := found.(*persistence.ProcessDefinitionDoc)
		*doc = *expected
	case *persistence.ProcessLaunchDoc:
		expected := found.(*persistence.ProcessLaunchDoc)
		*doc = *expected
	case *persistence.ProcessDoc:
		expected := found.(*persistence.ProcessDoc)
		*doc = *expected
	default:
		panic(doc)
	}

	return nil
}

func (sp fakeStatePersistence) All(collName string, query, docs interface{}) error {
	sp.AddCall("All", collName, query, docs)
	if err := sp.NextErr(); err != nil {
		return errors.Trace(err)
	}

	var ids []string
	for _, id := range query.(bson.M)["$in"].([]string) {
		switch {
		case !strings.HasPrefix(id, "/"):
			ids = append(ids, id)
		case strings.HasPrefix(id, "/^c#"):
			for _, id := range sp.definitionDocs {
				ids = append(ids, id)
			}
		case strings.HasSuffix(id, "#launch/"):
			for _, id := range sp.launchDocs {
				ids = append(ids, id)
			}
		default:
			for _, id := range sp.procDocs {
				ids = append(ids, id)
			}
		}
	}

	switch docs := docs.(type) {
	case *[]persistence.ProcessDefinitionDoc:
		var found []persistence.ProcessDefinitionDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*persistence.ProcessDefinitionDoc))
		}
		*docs = found
	case *[]persistence.ProcessLaunchDoc:
		var found []persistence.ProcessLaunchDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*persistence.ProcessLaunchDoc))
		}
		*docs = found
	case *[]persistence.ProcessDoc:
		var found []persistence.ProcessDoc
		for _, id := range ids {
			doc, ok := sp.docs[id]
			if !ok {
				continue
			}
			found = append(found, *doc.(*persistence.ProcessDoc))
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
