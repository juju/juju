// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/persistence"
)

var _ = gc.Suite(&procsPersistenceSuite{})

type procsPersistenceSuite struct {
	persistence.BaseSuite
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsCharmAndUnit(c *gc.C) {
	definitions := s.NewDefinitions("docker", "procA")
	s.SetUnit("a-unit/0")

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procA",
				CharmID:  "local:series/dummy-1",
				ProcName: "procA",
				DocKind:  "definition",
				UnitID:   "a-unit/0",
				Type:     "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsCharmOnly(c *gc.C) {
	definitions := s.NewDefinitions("docker", "procA")
	s.SetUnit("")

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procA",
				CharmID:  "local:series/dummy-1",
				ProcName: "procA",
				DocKind:  "definition",
				Type:     "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMultiple(c *gc.C) {
	definitions := s.NewDefinitions("docker", "procA", "procB")
	s.SetUnit("")

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procA",
				CharmID:  "local:series/dummy-1",
				ProcName: "procA",
				DocKind:  "definition",
				Type:     "docker",
			},
		}, {
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procB",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procB",
				CharmID:  "local:series/dummy-1",
				ProcName: "procB",
				DocKind:  "definition",
				Type:     "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsEmpty(c *gc.C) {
	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, gc.HasLen, 0)
	c.Check(mismatched, gc.HasLen, 0)
	s.Stub.CheckCallNames(c)
	s.State.CheckNoOps(c)
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	definitions := s.NewDefinitions("docker", "procA")
	s.SetUnit("")

	pp := s.NewPersistence()
	_, _, err := pp.EnsureDefinitions(definitions...)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsFound(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	definitions := s.NewDefinitions("docker", "procA")
	s.SetUnit("")
	expected := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		Type:     "docker",
	}
	s.State.SetDocs(expected)

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"procd#local:series/dummy-1#procA",
	})
	c.Check(mismatched, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "Run", "All")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: expected,
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMismatched(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	definitions := s.NewDefinitions("kvm", "procA")
	s.SetUnit("")
	doc := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		Type:     "docker",
	}
	s.State.SetDocs(doc)

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"procd#local:series/dummy-1#procA",
	})
	c.Check(mismatched, jc.DeepEquals, []string{
		"procd#local:series/dummy-1#procA",
	})
	s.Stub.CheckCallNames(c, "Run", "All")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procA",
				CharmID:  "local:series/dummy-1",
				ProcName: "procA",
				DocKind:  "definition",
				Type:     "kvm",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestEnsureDefininitionsMixed(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	definitions := s.NewDefinitions("kvm", "procA")
	definitions = append(definitions, s.NewDefinitions("docker", "procB", "procC")...)
	s.SetUnit("a-unit/0")
	doc := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		UnitID:   "a-unit/0",
		Type:     "docker",
	}
	expected := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procB",
		CharmID:  "local:series/dummy-1",
		ProcName: "procB",
		DocKind:  "definition",
		UnitID:   "a-unit/0",
		Type:     "docker",
	}
	s.State.SetDocs(doc, expected)

	pp := s.NewPersistence()
	found, mismatched, err := pp.EnsureDefinitions(definitions...)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.DeepEquals, []string{
		"procd#local:series/dummy-1#procA",
		"procd#local:series/dummy-1#procB",
	})
	c.Check(mismatched, jc.DeepEquals, []string{
		"procd#local:series/dummy-1#procA",
	})
	s.Stub.CheckCallNames(c, "Run", "All")
	s.State.CheckOps(c, [][]txn.Op{{
		// first attempt
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procA",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procA",
				CharmID:  "local:series/dummy-1",
				ProcName: "procA",
				DocKind:  "definition",
				UnitID:   "a-unit/0",
				Type:     "kvm",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procB",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procB",
				CharmID:  "local:series/dummy-1",
				ProcName: "procB",
				DocKind:  "definition",
				UnitID:   "a-unit/0",
				Type:     "docker",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procC",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procC",
				CharmID:  "local:series/dummy-1",
				ProcName: "procC",
				DocKind:  "definition",
				UnitID:   "a-unit/0",
				Type:     "docker",
			},
		},
	}, {
		// second attempt
		{
			C:      "workloadprocesses",
			Id:     "procd#local:series/dummy-1#procC",
			Assert: txn.DocMissing,
			Insert: &persistence.DefinitionDoc{
				DocID:    "procd#local:series/dummy-1#procC",
				CharmID:  "local:series/dummy-1",
				ProcName: "procC",
				DocKind:  "definition",
				UnitID:   "a-unit/0",
				Type:     "docker",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertOkay(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]

	pp := s.NewPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.LaunchDoc{
				DocID:     "proc#a-unit/0#procA/procA-xyz#launch",
				UnitID:    "a-unit/0",
				ProcName:  "procA",
				PluginID:  "procA-xyz",
				DocKind:   "launch",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "proc#a-unit/0#procA/procA-xyz",
				UnitID:       "a-unit/0",
				ProcName:     "procA",
				PluginID:     "procA-xyz",
				DocKind:      "process",
				PluginStatus: "running",
			},
		},
		// TODO(ericsnow) This op will be there once we add definitions.
		//{
		//	C:      "workloadprocesses",
		//	Id:     "procd#local:series/dummy-1#procA",
		//	Assert: txn.DocMissing,
		//	Insert: &persistence.DefinitionDoc{
		//		DocID: "procd#local:series/dummy-1#procA",
		//		CharmID:  "local:series/dummy-1",
		//		ProcName: "procA",
		//      DocKind: "definition",
		//		Type:  "docker",
		//	},
		//},
	}})
}

func (s *procsPersistenceSuite) TestInsertDefinitionExists(c *gc.C) {
	expected := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		Type:     "docker",
	}
	s.State.SetDocs(expected)
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]

	pp := s.NewPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.LaunchDoc{
				DocID:     "proc#a-unit/0#procA/procA-xyz#launch",
				UnitID:    "a-unit/0",
				ProcName:  "procA",
				PluginID:  "procA-xyz",
				DocKind:   "launch",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "proc#a-unit/0#procA/procA-xyz",
				UnitID:       "a-unit/0",
				ProcName:     "procA",
				PluginID:     "procA-xyz",
				DocKind:      "process",
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertDefinitionMismatch(c *gc.C) {
	expected := &persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		Type:     "docker",
	}
	s.State.SetDocs(expected)
	proc := s.NewProcesses("kvm", "procA/procA-xyz")[0]

	pp := s.NewPersistence()
	okay, err := pp.Insert(proc)
	// TODO(ericsnow) Should this fail instead?
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.LaunchDoc{
				DocID:     "proc#a-unit/0#procA/procA-xyz#launch",
				UnitID:    "a-unit/0",
				ProcName:  "procA",
				PluginID:  "procA-xyz",
				DocKind:   "launch",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "proc#a-unit/0#procA/procA-xyz",
				UnitID:       "a-unit/0",
				ProcName:     "procA",
				PluginID:     "procA-xyz",
				DocKind:      "process",
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertAlreadyExists(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
	s.SetDocs(proc)
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	okay, err := pp.Insert(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz#launch",
			Assert: txn.DocMissing,
			Insert: &persistence.LaunchDoc{
				DocID:     "proc#a-unit/0#procA/procA-xyz#launch",
				UnitID:    "a-unit/0",
				ProcName:  "procA",
				PluginID:  "procA-xyz",
				DocKind:   "launch",
				RawStatus: "running",
			},
		},
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:        "proc#a-unit/0#procA/procA-xyz",
				UnitID:       "a-unit/0",
				ProcName:     "procA",
				PluginID:     "procA-xyz",
				DocKind:      "process",
				PluginStatus: "running",
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	proc := s.NewProcesses("docker", "procA")[0]

	pp := s.NewPersistence()
	_, err := pp.Insert(proc)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
	s.SetDocs(proc)
	newStatus := process.Status{Label: "still running"}

	pp := s.NewPersistence()
	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"pluginstatus", "still running"}}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	newStatus := process.Status{Label: "still running"}

	pp := s.NewPersistence()
	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run", "One")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"pluginstatus", "still running"}}},
			},
		},
	}})
}

//func (s *procsPersistenceSuite) TestSetStatusDying(c *gc.C) {
//	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
//	docs := s.SetDocs(proc)
//	docs[0].Proc.Life = persistence.Dying
//	s.Stub.SetErrors(txn.ErrAborted)
//	newStatus := process.Status{Label: "still running"}
//
//	pp := s.NewPersistence()
//	okay, err := pp.SetStatus("procA/procA-xyz", newStatus)
//	c.Assert(err, jc.ErrorIsNil)
//
//	c.Check(okay, jc.IsTrue)
//	s.Stub.CheckCallNames(c, "Run", "One")
//	s.State.CheckOps(c, [][]txn.Op{{
//		{
//			C:      "workloadprocesses",
//			Id:     "proc#a-unit/0#procA/procA-xyz",
//			Assert: txn.DocExists,
//			Update: bson.D{
//				{"$set", bson.D{{"pluginstatus", "still running"}}},
//			},
//		},
//	}})
//}

func (s *procsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
	s.SetDocs(proc)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.SetStatus("procA/procA-xyz", process.Status{Label: "still running"})

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestListOkay(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	c.Check(procs, jc.DeepEquals, []process.Info{existing[0]})
	c.Check(missing, gc.HasLen, 0)
}

func (s *procsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	procs, missing, err := pp.List("procB/abc", "procC/123")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	c.Check(procs, jc.DeepEquals, []process.Info{existing[1]})
	c.Check(missing, jc.DeepEquals, []string{"procC/123"})
}

func (s *procsPersistenceSuite) TestListEmpty(c *gc.C) {
	pp := s.NewPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	c.Check(procs, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{"procA/xyz"})
}

func (s *procsPersistenceSuite) TestListInconsistent(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)
	s.RemoveDoc(existing[0], "launch")

	pp := s.NewPersistence()
	_, _, err := pp.List("procA/xyz")

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestListFailure(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, _, err := pp.List()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestListAllOkay(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	sort.Sort(byName(procs))
	sort.Sort(byName(existing))
	c.Check(procs, jc.DeepEquals, existing)
}

func (s *procsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	pp := s.NewPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	c.Check(procs, gc.HasLen, 0)
}

func (s *procsPersistenceSuite) TestListAllIncludeCharmDefined(c *gc.C) {
	s.State.SetDocs(&persistence.DefinitionDoc{
		DocID:    "procd#local:series/dummy-1#procA",
		CharmID:  "local:series/dummy-1",
		ProcName: "procA",
		DocKind:  "definition",
		Type:     "docker",
	})
	existing := s.NewProcesses("docker", "procB/abc", "procC/xyz")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "All")
	s.State.CheckNoOps(c)
	existing = append(existing, process.Info{
		Process: charm.Process{
			Name: "procA",
			Type: "docker",
		},
	})
	sort.Sort(byName(procs))
	sort.Sort(byName(existing))
	c.Check(procs, jc.DeepEquals, existing)
}

type byName []process.Info

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].Name < b[j].Name }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *procsPersistenceSuite) TestListAllInconsistent(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)
	s.RemoveDoc(existing[0], "launch")

	pp := s.NewPersistence()
	_, err := pp.ListAll()

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestRemoveOkay(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/xyz")[0]
	s.SetDocs(proc)

	pp := s.NewPersistence()
	found, err := pp.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/xyz#launch",
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *procsPersistenceSuite) TestRemoveMissing(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	found, err := pp.Remove("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run", "One", "One", "One")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/xyz#launch",
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

//func (s *procsPersistenceSuite) TestRemoveDying(c *gc.C) {
//	proc := s.NewProcesses("docker", "procA/xyz")[0]
//	docs := s.SetDocs(proc)
//	docs[0].Proc.Life = persistence.Dying
//
//	pp := s.NewPersistence()
//	found, err := pp.Remove("procA/xyz")
//	c.Assert(err, jc.ErrorIsNil)
//
//	c.Check(found, jc.IsTrue)
//	s.Stub.CheckCallNames(c, "Run")
//	s.State.CheckOps(c, [][]txn.Op{{
//		{
//			C:      "workloadprocesses",
//			Id:     "proc#a-unit/0#procA/xyz#launch",
//			Assert: txn.DocExists,
//			Remove: true,
//		}, {
//			C:      "workloadprocesses",
//			Id:     "proc#a-unit/0#procA/xyz",
//			Assert: txn.DocExists,
//			Remove: true,
//		},
//	}})
//}

func (s *procsPersistenceSuite) TestRemoveInconsistent(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/xyz")[0]
	s.SetDocs(proc)
	s.RemoveDoc(proc, "launch")
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	_, err := pp.Remove("procA/xyz")

	c.Check(err, gc.ErrorMatches, "found inconsistent records .*")
}

func (s *procsPersistenceSuite) TestRemoveFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.Remove("procA/xyz")

	c.Check(errors.Cause(err), gc.Equals, failure)
}
