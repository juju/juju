// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/persistence"
)

var _ = gc.Suite(&procsPersistenceSuite{})

type procsPersistenceSuite struct {
	persistence.BaseSuite
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
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:  "proc#a-unit/0#procA/procA-xyz",
				UnitID: "a-unit/0",

				Name: "procA",
				Type: "docker",

				PluginID:       "procA-xyz",
				OriginalStatus: "running",

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
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.ProcessDoc{
				DocID:  "proc#a-unit/0#procA/procA-xyz",
				UnitID: "a-unit/0",

				Name: "procA",
				Type: "docker",

				PluginID:       "procA-xyz",
				OriginalStatus: "running",

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

func newStatusInfo(id, state, message, pluginStatus string) process.Info {
	info := process.Info{
		Status: process.Status{
			State:   state,
			Message: message,
		},
	}
	info.Name, info.Details.ID = process.ParseID(id)
	info.Details.Status.Label = pluginStatus
	return info
}

func (s *procsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
	s.SetDocs(proc)
	info := newStatusInfo(proc.ID(), process.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(info)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", process.StateRunning},
					{"failed", false},
					{"error", false},
					{"status", "good to go"},
					{"pluginstatus", "still running"},
				}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	info := newStatusInfo("procA/procA-xyz", process.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(info)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/procA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", process.StateRunning},
					{"failed", false},
					{"error", false},
					{"status", "good to go"},
					{"pluginstatus", "still running"},
				}},
			},
		},
	}})
}

func (s *procsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	proc := s.NewProcesses("docker", "procA/procA-xyz")[0]
	s.SetDocs(proc)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	info := newStatusInfo(proc.ID(), process.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	_, err := pp.SetStatus(info)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *procsPersistenceSuite) TestListOkay(c *gc.C) {
	existing := s.NewProcesses("docker", "procA/xyz", "procB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
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

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(procs, jc.DeepEquals, []process.Info{existing[1]})
	c.Check(missing, jc.DeepEquals, []string{"procC/123"})
}

func (s *procsPersistenceSuite) TestListEmpty(c *gc.C) {
	pp := s.NewPersistence()
	procs, missing, err := pp.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(procs, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{"procA/xyz"})
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

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	sort.Sort(byName(procs))
	sort.Sort(byName(existing))
	c.Check(procs, jc.DeepEquals, existing)
}

func (s *procsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	pp := s.NewPersistence()
	procs, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(procs, gc.HasLen, 0)
}

type byName []process.Info

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].Name < b[j].Name }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

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
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloadprocesses",
			Id:     "proc#a-unit/0#procA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *procsPersistenceSuite) TestRemoveFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.Remove("procA/xyz")

	c.Check(errors.Cause(err), gc.Equals, failure)
}
