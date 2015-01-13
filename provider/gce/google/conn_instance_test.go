// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

func (s *connSuite) TestConnectionInstance(c *gc.C) {
	inst, err := google.ConnInstance(s.conn, "a-zone", "inst-id")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst, jc.DeepEquals, &s.RawInstance)
}

func (s *connSuite) TestConnectionInstanceFailed(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")
	_, err := google.ConnInstance(s.conn, "a-zone", "inst-id")

	c.Check(errors.Cause(err), gc.Equals, s.DoCallErr)
}

func (s *connSuite) TestConnectionAddInstance(c *gc.C) {
	s.RawInstance.Zone = "a-zone"
	s.RawInstance.MachineType = "mtype"
	var zoneArg, idArg string
	s.PatchValue(google.RawInstance, func(conn *google.Connection, zone, id string) (*compute.Instance, error) {
		zoneArg = zone
		idArg = id
		return &s.RawInstance, nil
	})
	s.setNoOpWait()

	inst := google.InstanceSpecRaw(s.InstanceSpec)
	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.conn, inst, "mtype", zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst, jc.DeepEquals, &s.RawInstance)
	c.Check(zoneArg, gc.Equals, "a-zone")
	c.Check(idArg, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionAddInstanceFailed(c *gc.C) {
	s.DoCallErr = errors.New("unknown")
	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.conn, &s.RawInstance, "mtype", zones)

	c.Check(errors.Cause(err), gc.Equals, s.DoCallErr)
}

func (s *connSuite) TestConnectionAddInstanceGetFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.PatchValue(google.RawInstance, func(*google.Connection, string, string) (*compute.Instance, error) {
		return nil, failure
	})
	s.setNoOpWait()

	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.conn, &s.RawInstance, "mtype", zones)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionInstances(c *gc.C) {
	s.instanceList = &compute.InstanceAggregatedList{
		Items: map[string]compute.InstancesScopedList{
			"": compute.InstancesScopedList{
				Instances: []*compute.Instance{&s.RawInstance},
			},
		},
	}

	insts, err := s.conn.Instances("sp", google.StatusRunning)
	c.Assert(err, jc.ErrorIsNil)

	google.SetInstanceSpec(&s.Instance, nil)
	c.Check(insts, jc.DeepEquals, []google.Instance{s.Instance})
}

func (s *connSuite) TestConnectionInstancesFailure(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")
	_, err := s.conn.Instances("sp", google.StatusRunning)

	c.Check(errors.Cause(err), gc.Equals, s.DoCallErr)
}

func (s *connSuite) TestConnectionInstancesMultipage(c *gc.C) {
	s.instanceList = &compute.InstanceAggregatedList{
		Items: map[string]compute.InstancesScopedList{
			"": compute.InstancesScopedList{
				Instances: []*compute.Instance{&s.RawInstance},
			},
		},
		NextPageToken: "token",
	}
	s.PatchValue(google.InstsNextPage, func(call *compute.InstancesAggregatedListCall, _ string) *compute.InstancesAggregatedListCall {
		s.instanceList = &compute.InstanceAggregatedList{
			Items: map[string]compute.InstancesScopedList{
				"": compute.InstancesScopedList{
					Instances: []*compute.Instance{&s.RawInstance},
				},
			},
		}
		return call
	})

	insts, err := s.conn.Instances("sp", google.StatusRunning)
	c.Assert(err, jc.ErrorIsNil)

	google.SetInstanceSpec(&s.Instance, nil)
	c.Check(insts, jc.DeepEquals, []google.Instance{s.Instance, s.Instance})
}

func (s *connSuite) TestConnectionRemoveInstance(c *gc.C) {
	s.PatchValue(google.ConnRemoveFirewall, func(*google.Connection, string) error {
		return nil
	})
	s.setNoOpWait()

	err := google.ConnRemoveInstance(s.conn, "spam", "a-zone")

	c.Check(err, jc.ErrorIsNil)
}

func (s *connSuite) TestConnectionRemoveInstanceFailed(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")

	err := google.ConnRemoveInstance(s.conn, "spam", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, s.DoCallErr)
}

func (s *connSuite) TestConnectionRemoveInstanceFirewall(c *gc.C) {
	var fwArg string
	s.PatchValue(google.ConnRemoveFirewall, func(_ *google.Connection, fw string) error {
		fwArg = fw
		return nil
	})
	s.setNoOpWait()

	err := google.ConnRemoveInstance(s.conn, "spam", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fwArg, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstances(c *gc.C) {
}

func (s *connSuite) TestConnectionRemoveInstancesPartialList(c *gc.C) {
}

func (s *connSuite) TestConnectionRemoveInstancesListFailed(c *gc.C) {
}

func (s *connSuite) TestConnectionRemoveInstancesRemoveFailed(c *gc.C) {
}
