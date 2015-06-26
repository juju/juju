// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

func (s *connSuite) TestConnectionSimpleAddInstance(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst := &s.RawInstance
	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.Conn, inst, "mtype", zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst, jc.DeepEquals, &s.RawInstanceFull)
}

func (s *connSuite) TestConnectionSimpleAddInstanceAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull
	expected := s.RawInstance
	expected.MachineType = "zones/a-zone/machineTypes/mtype"

	inst := &s.RawInstance
	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.Conn, inst, "mtype", zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AddInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[0].InstValue, gc.DeepEquals, expected)
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[1].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, gc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[1].ID, gc.Equals, "spam")
}

func (s *instanceSuite) TestConnectionAddInstance(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst, err := s.Conn.AddInstance(s.InstanceSpec, "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.ZoneName, gc.Equals, "a-zone")
	c.Check(inst.Status(), gc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), jc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), jc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(inst)
	c.Check(spec, gc.DeepEquals, &s.InstanceSpec)
}

func (s *instanceSuite) TestConnectionAddInstanceAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	_, err := s.Conn.AddInstance(s.InstanceSpec, "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AddInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	// We check s.FakeConn.Calls[0].InstValue below.
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[1].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, gc.Equals, "a-zone")

	metadata := compute.Metadata{Items: []*compute.MetadataItems{{
		Key:   "eggs",
		Value: "steak",
	}}}
	networkInterfaces := []*compute.NetworkInterface{{
		Network: "global/networks/somenetwork",
		AccessConfigs: []*compute.AccessConfig{{
			Name: "somenetif",
			Type: "ONE_TO_ONE_NAT",
		}},
	}}
	attachedDisks := []*compute.AttachedDisk{{
		Type:       "PERSISTENT",
		Boot:       true,
		Mode:       "READ_WRITE",
		AutoDelete: true,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskSizeGb:  15,
			SourceImage: "some/image/path",
		},
	}}
	c.Check(s.FakeConn.Calls[0].InstValue, gc.DeepEquals, compute.Instance{
		Name:              "spam",
		MachineType:       "zones/a-zone/machineTypes/mtype",
		Disks:             attachedDisks,
		NetworkInterfaces: networkInterfaces,
		Metadata:          &metadata,
		Tags:              &compute.Tags{Items: []string{"spam"}},
	})
}

func (s *connSuite) TestConnectionAddInstanceFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure

	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", zones)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionAddInstanceWaitFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := s.NewWaitError(nil, errors.New("unknown"))
	s.FakeConn.Err = failure

	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", zones)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionAddInstanceGetFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 1

	zones := []string{"a-zone"}
	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", zones)

	c.Check(errors.Cause(err), gc.Equals, failure)
	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
}

func (s *connSuite) TestConnectionInstance(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst, err := s.Conn.Instance("spam", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.ZoneName, gc.Equals, "a-zone")
	c.Check(inst.Status(), gc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), jc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), jc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(&inst)
	c.Check(spec, gc.IsNil)
}

func (s *connSuite) TestConnectionInstanceAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	_, err := s.Conn.Instance("ham", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ID, gc.Equals, "ham")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
}

func (s *connSuite) TestConnectionInstanceFail(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	_, err := s.Conn.Instance("spam", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionInstances(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	insts, err := s.Conn.Instances("sp", google.StatusRunning)
	c.Assert(err, jc.ErrorIsNil)

	google.SetInstanceSpec(&s.Instance, nil)
	c.Check(insts, jc.DeepEquals, []google.Instance{s.Instance})
}

func (s *connSuite) TestConnectionInstancesFailure(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	_, err := s.Conn.Instances("sp", google.StatusRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstance(c *gc.C) {
	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(err, jc.ErrorIsNil)
}

func (s *connSuite) TestConnectionRemoveInstanceAPI(c *gc.C) {
	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[0].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[1].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].Name, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstanceFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstanceFirewallFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 1

	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, failure)
	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
}

func (s *connSuite) TestConnectionRemoveInstances(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(err, jc.ErrorIsNil)
}

func (s *connSuite) TestConnectionRemoveInstancesAPI(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.RemoveInstances("sp", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstancesMultiple(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{
		&s.RawInstanceFull,
		&compute.Instance{
			Name: "special",
			Zone: "a-zone",
		},
	}

	err := s.Conn.RemoveInstances("", "spam", "special")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 5)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[3].FuncName, gc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[3].ID, gc.Equals, "special")
	c.Check(s.FakeConn.Calls[4].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[4].Name, gc.Equals, "special")
}

func (s *connSuite) TestConnectionRemoveInstancesPartialMatch(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{
		&s.RawInstanceFull,
		&compute.Instance{
			Name: "special",
			Zone: "a-zone",
		},
	}

	err := s.Conn.RemoveInstances("", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, gc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, gc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstancesListFailed(c *gc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstancesRemoveFailed(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 2

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(err, gc.ErrorMatches, ".*some instance removals failed: .*")
}
