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

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/persistence"
)

var _ = gc.Suite(&workloadsPersistenceSuite{})

type workloadsPersistenceSuite struct {
	persistence.BaseSuite
}

func (s *workloadsPersistenceSuite) TestTrackOkay(c *gc.C) {
	wl := s.NewWorkloads("docker", "workloadA/workloadA-xyz")[0]

	wp := s.NewPersistence()
	okay, err := wp.Track(wl)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/workloadA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.WorkloadDoc{
				DocID:  "workload#a-unit/0#workloadA/workloadA-xyz",
				UnitID: "a-unit/0",

				Name: "workloadA",
				Type: "docker",

				PluginID:       "workloadA-xyz",
				OriginalStatus: "running",

				PluginStatus: "running",
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestTrackAlreadyExists(c *gc.C) {
	wl := s.NewWorkloads("docker", "workloadA/workloadA-xyz")[0]
	s.SetDocs(wl)
	s.Stub.SetErrors(txn.ErrAborted)

	wp := s.NewPersistence()
	okay, err := wp.Track(wl)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/workloadA-xyz",
			Assert: txn.DocMissing,
			Insert: &persistence.WorkloadDoc{
				DocID:  "workload#a-unit/0#workloadA/workloadA-xyz",
				UnitID: "a-unit/0",

				Name: "workloadA",
				Type: "docker",

				PluginID:       "workloadA-xyz",
				OriginalStatus: "running",

				PluginStatus: "running",
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestTrackFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	wl := s.NewWorkloads("docker", "workloadA")[0]

	pp := s.NewPersistence()
	_, err := pp.Track(wl)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func newStatusInfo(state, message, pluginStatus string) workload.CombinedStatus {
	return workload.CombinedStatus{
		Status: workload.Status{
			State:   state,
			Message: message,
		},
		PluginStatus: workload.PluginStatus{
			State: pluginStatus,
		},
	}
}

func (s *workloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	wl := s.NewWorkloads("docker", "workloadA/workloadA-xyz")[0]
	s.SetDocs(wl)
	status := newStatusInfo(workload.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(wl.ID(), status)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/workloadA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", workload.StateRunning},
					{"blocker", ""},
					{"status", "good to go"},
					{"pluginstatus", "still running"},
				}},
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)
	status := newStatusInfo(workload.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	okay, err := pp.SetStatus("workloadA/workloadA-xyz", status)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/workloadA-xyz",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", workload.StateRunning},
					{"blocker", ""},
					{"status", "good to go"},
					{"pluginstatus", "still running"},
				}},
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	wl := s.NewWorkloads("docker", "workloadA/workloadA-xyz")[0]
	s.SetDocs(wl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	status := newStatusInfo(workload.StateRunning, "good to go", "still running")

	pp := s.NewPersistence()
	_, err := pp.SetStatus(wl.ID(), status)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *workloadsPersistenceSuite) TestListOkay(c *gc.C) {
	existing := s.NewWorkloads("docker", "workloadA/xyz", "workloadB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	workloads, missing, err := pp.List("workloadA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, jc.DeepEquals, []workload.Info{existing[0]})
	c.Check(missing, gc.HasLen, 0)
}

func (s *workloadsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	existing := s.NewWorkloads("docker", "workloadA/xyz", "workloadB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	workloads, missing, err := pp.List("workloadB/abc", "workloadC/123")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, jc.DeepEquals, []workload.Info{existing[1]})
	c.Check(missing, jc.DeepEquals, []string{"workloadC/123"})
}

func (s *workloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	pp := s.NewPersistence()
	workloads, missing, err := pp.List("workloadA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{"workloadA/xyz"})
}

func (s *workloadsPersistenceSuite) TestListFailure(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, _, err := pp.List()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *workloadsPersistenceSuite) TestListAllOkay(c *gc.C) {
	existing := s.NewWorkloads("docker", "workloadA/xyz", "workloadB/abc")
	s.SetDocs(existing...)

	pp := s.NewPersistence()
	workloads, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	sort.Sort(byName(workloads))
	sort.Sort(byName(existing))
	c.Check(workloads, jc.DeepEquals, existing)
}

func (s *workloadsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	pp := s.NewPersistence()
	workloads, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, gc.HasLen, 0)
}

type byName []workload.Info

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].Name < b[j].Name }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *workloadsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *workloadsPersistenceSuite) TestUntrackOkay(c *gc.C) {
	wl := s.NewWorkloads("docker", "workloadA/xyz")[0]
	s.SetDocs(wl)

	pp := s.NewPersistence()
	found, err := pp.Untrack("workloadA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *workloadsPersistenceSuite) TestUntrackMissing(c *gc.C) {
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	found, err := pp.Untrack("workloadA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#workloadA/xyz",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *workloadsPersistenceSuite) TestUntrackFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.Untrack("workloadA/xyz")

	c.Check(errors.Cause(err), gc.Equals, failure)
}
