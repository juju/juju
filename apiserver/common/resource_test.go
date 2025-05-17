// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"sync"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
)

type resourceSuite struct{}

func TestResourceSuite(t *stdtesting.T) {
	tc.Run(t, &resourceSuite{})
}

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Kill() {
	r.stopped = true
}

func (r *fakeResource) Wait() error {
	return nil
}

func (resourceSuite) TestRegisterGetCount(c *tc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	id := rs.Register(r1)
	c.Assert(id, tc.Equals, "1")
	c.Assert(rs.Get("1"), tc.Equals, r1)
	c.Assert(rs.Count(), tc.Equals, 1)

	r2 := &fakeResource{}
	id = rs.Register(r2)
	c.Assert(id, tc.Equals, "2")
	c.Assert(rs.Get("2"), tc.Equals, r2)
	c.Assert(rs.Count(), tc.Equals, 2)
}

func (resourceSuite) TestRegisterNamedGetCount(c *tc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("fake1", r1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rs.Count(), tc.Equals, 1)
	c.Check(rs.Get("fake1"), tc.Equals, r1)
}

func (resourceSuite) TestRegisterNamedRepeatedName(c *tc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	r2 := &fakeResource{}
	err := rs.RegisterNamed("fake1", r1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rs.Count(), tc.Equals, 1)
	err = rs.RegisterNamed("fake1", r2)
	c.Check(err, tc.ErrorMatches, `resource "fake1" already registered`)
	c.Check(rs.Count(), tc.Equals, 1)
	c.Check(rs.Get("fake1"), tc.Equals, r1)
}

func (resourceSuite) TestRegisterNamedIntegerName(c *tc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("1", r1)
	c.Check(err, tc.ErrorMatches, `RegisterNamed does not allow integer names: "1"`)
	c.Check(rs.Count(), tc.Equals, 0)
	c.Check(rs.Get("fake1"), tc.IsNil)
}

func (resourceSuite) TestRegisterNamedIntegerStart(c *tc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("1fake", r1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rs.Count(), tc.Equals, 1)
	c.Check(rs.Get("1fake"), tc.Equals, r1)
}

func (resourceSuite) TestConcurrency(c *tc.C) {
	// This test is designed to cause the race detector
	// to fail if the locking is not done correctly.
	var wg sync.WaitGroup
	rs := common.NewResources()

	start := func(f func()) {
		wg.Add(1)
		go func() {
			f()
			wg.Done()
		}()
	}
	rs.Register(&fakeResource{})
	start(func() {
		rs.Register(&fakeResource{})
	})
	start(func() {
		rs.RegisterNamed("named", &fakeResource{})
	})
	start(func() {
		rs.Stop("1")
	})
	start(func() {
		rs.Count()
	})
	start(func() {
		rs.StopAll()
	})
	start(func() {
		rs.Get("2")
	})
	start(func() {
		rs.Get("named")
	})
	wg.Wait()
}

func (resourceSuite) TestStop(c *tc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.Stop("1")
	c.Assert(r1.stopped, tc.IsTrue)
	c.Assert(rs.Get("1"), tc.IsNil)
	c.Assert(r2.stopped, tc.IsFalse)
	c.Assert(rs.Get("2"), tc.Equals, r2)
	c.Assert(rs.Count(), tc.Equals, 1)
}

func (resourceSuite) TestStopAll(c *tc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.StopAll()

	c.Assert(r1.stopped, tc.IsTrue)
	c.Assert(rs.Get("1"), tc.IsNil)
	c.Assert(r2.stopped, tc.IsTrue)
	c.Assert(rs.Get("2"), tc.IsNil)

	c.Assert(rs.Count(), tc.Equals, 0)
}
