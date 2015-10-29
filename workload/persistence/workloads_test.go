// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"fmt"
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
	wl := s.NewWorkload("docker", "workloadA/workloadA-xyz")

	wp := s.NewPersistence()
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	okay, err := wp.Track(id, wl)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "All", "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocMissing,
			Insert: &persistence.WorkloadDoc{
				DocID:  "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
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
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	wl := s.NewWorkload("docker", "workloadA/workloadA-xyz")
	s.SetDoc(id, wl)
	s.Stub.SetErrors(nil, txn.ErrAborted)

	wp := s.NewPersistence()
	okay, err := wp.Track(id, wl)

	c.Check(okay, jc.IsFalse)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	s.Stub.CheckCallNames(c, "All")
	s.State.CheckOps(c, nil)
}

func (s *workloadsPersistenceSuite) TestTrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(nil, failure)
	wl := s.NewWorkload("docker", "workloadA")

	pp := s.NewPersistence()
	_, err := pp.Track(id, wl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "All", "Run")
}

func (s *workloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	wl := s.NewWorkload("docker", "workloadA/workloadA-xyz")
	s.SetDoc(id, wl)

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(id, workload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", workload.StateRunning},
					{"status", workload.StateRunning},
					{"pluginstatus", workload.StateRunning},
				}},
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(id, workload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", workload.StateRunning},
					{"status", workload.StateRunning},
					{"pluginstatus", workload.StateRunning},
				}},
			},
		},
	}})
}

func (s *workloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	wl := s.NewWorkload("docker", "workloadA/workloadA-xyz")
	s.SetDoc(id, wl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.SetStatus(id, workload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *workloadsPersistenceSuite) TestListOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	wl := s.NewWorkload("docker", "workloadA/xyz")
	s.SetDoc(id, wl)
	other := s.NewWorkload("docker", "workloadB/abc")
	s.SetDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	pp := s.NewPersistence()
	workloads, missing, err := pp.List(id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, jc.DeepEquals, []workload.Info{wl})
	c.Check(missing, gc.HasLen, 0)
}

func (s *workloadsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	wl := s.NewWorkload("docker", "workloadB/abc")
	s.SetDoc(id, wl)
	other := s.NewWorkload("docker", "workloadA/xyz")
	s.SetDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	missingID := "f47ac10b-58cc-4372-a567-0e02b2c3d481"
	pp := s.NewPersistence()
	workloads, missing, err := pp.List(id, missingID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, jc.DeepEquals, []workload.Info{wl})
	c.Check(missing, jc.DeepEquals, []string{missingID})
}

func (s *workloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pp := s.NewPersistence()
	workloads, missing, err := pp.List(id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(workloads, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{id})
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
	for i, wl := range existing {
		s.SetDoc(fmt.Sprintf("%d", i), wl)
	}

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
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	wl := s.NewWorkload("docker", "workloadA/xyz")
	s.SetDoc(id, wl)

	pp := s.NewPersistence()
	found, err := pp.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *workloadsPersistenceSuite) TestUntrackMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	found, err := pp.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "workloads",
			Id:     "workload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *workloadsPersistenceSuite) TestUntrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.Untrack(id)

	c.Check(errors.Cause(err), gc.Equals, failure)
}
