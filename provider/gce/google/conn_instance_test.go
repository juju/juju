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
	err := google.ConnAddInstance(s.Conn, inst, "mtype", "a-zone")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst, jc.DeepEquals, &s.RawInstanceFull)
}

func (s *connSuite) TestConnectionSimpleAddInstanceAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull
	expected := s.RawInstance
	expected.MachineType = "zones/a-zone/machineTypes/mtype"

	inst := &s.RawInstance
	err := google.ConnAddInstance(s.Conn, inst, "mtype", "a-zone")
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

	inst, err := s.Conn.AddInstance(s.InstanceSpec)
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

	_, err := s.Conn.AddInstance(s.InstanceSpec)
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
		Value: google.StringPtr("steak"),
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
		ServiceAccounts: []*compute.ServiceAccount{{
			Email: "fred@foo.com",
		}},
	})
}

func (s *connSuite) TestConnectionAddInstanceFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *connSuite) TestConnectionAddInstanceWaitFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	cause := errors.New("unknown")
	failure := s.NewWaitError(nil, cause)
	s.FakeConn.Err = failure

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

	c.Check(errors.Cause(err), gc.Equals, cause)
}

func (s *connSuite) TestConnectionAddInstanceGetFailed(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 1

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

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
		{
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
		{
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

func (s *connSuite) TestUpdateMetadataNewAttribute(c *gc.C) {
	// Ensure we extract the name from the URL we get on the raw instance.
	s.RawInstanceFull.Zone = "http://eels/lone/wolf/a-zone"
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.UpdateMetadata("business", "time", s.RawInstanceFull.Name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.ProjectID, gc.Equals, "spam")
	c.Check(call.ZoneName, gc.Equals, "a-zone")
	c.Check(call.InstanceId, gc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, gc.Equals, "heymumwatchthis")
	c.Assert(md.Items, gc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "steak")
	checkMetadataItems(c, md.Items[1], "business", "time")
}

func (s *connSuite) TestUpdateMetadataExistingAttribute(c *gc.C) {
	// Ensure we extract the name from the URL we get on the raw instance.
	s.RawInstanceFull.Zone = "http://eels/lone/wolf/a-zone"
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.UpdateMetadata("eggs", "beans", s.RawInstanceFull.Name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.ProjectID, gc.Equals, "spam")
	c.Check(call.ZoneName, gc.Equals, "a-zone")
	c.Check(call.InstanceId, gc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, gc.Equals, "heymumwatchthis")
	c.Assert(md.Items, gc.HasLen, 1)
	checkMetadataItems(c, md.Items[0], "eggs", "beans")
}

func (s *connSuite) TestUpdateMetadataMultipleInstances(c *gc.C) {
	// Ensure we extract the name from the URL we get on the raw instance.
	s.RawInstanceFull.Zone = "http://eels/lone/wolf/a-zone"

	instance2 := s.RawInstanceFull
	instance2.Name = "trucks"
	instance2.Metadata = &compute.Metadata{
		Fingerprint: "faroffalienplanet",
		Items: []*compute.MetadataItems{
			makeMetadataItems("eggs", "beans"),
			makeMetadataItems("rick", "moranis"),
		},
	}

	instance3 := s.RawInstanceFull
	instance3.Name = "boats"
	instance3.Metadata = &compute.Metadata{
		Fingerprint: "imprisoned",
		Items: []*compute.MetadataItems{
			makeMetadataItems("eggs", "milk"),
			makeMetadataItems("rick", "moranis"),
		},
	}

	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull, &instance2, &instance3}

	err := s.Conn.UpdateMetadata("rick", "morty", "spam", "boats")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.ProjectID, gc.Equals, "spam")
	c.Check(call.ZoneName, gc.Equals, "a-zone")
	c.Check(call.InstanceId, gc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, gc.Equals, "heymumwatchthis")
	c.Assert(md.Items, gc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "steak")
	checkMetadataItems(c, md.Items[1], "rick", "morty")

	call = s.FakeConn.Calls[2]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.ProjectID, gc.Equals, "spam")
	c.Check(call.ZoneName, gc.Equals, "a-zone")
	c.Check(call.InstanceId, gc.Equals, "boats")

	md = call.Metadata
	c.Check(md.Fingerprint, gc.Equals, "imprisoned")
	c.Assert(md.Items, gc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "milk")
	checkMetadataItems(c, md.Items[1], "rick", "morty")
}

func (s *connSuite) TestUpdateMetadataError(c *gc.C) {
	instance2 := s.RawInstanceFull
	instance2.Name = "trucks"
	instance2.Metadata = &compute.Metadata{
		Fingerprint: "faroffalienplanet",
		Items: []*compute.MetadataItems{
			makeMetadataItems("eggs", "beans"),
			makeMetadataItems("rick", "moranis"),
		},
	}
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull, &instance2}
	s.FakeConn.Err = errors.New("kablooey")
	s.FakeConn.FailOnCall = 1

	err := s.Conn.UpdateMetadata("rick", "morty", "spam", "trucks")
	c.Assert(err, gc.ErrorMatches, `some metadata updates failed: \[spam\]`)

	c.Assert(s.FakeConn.Calls, gc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.InstanceId, gc.Equals, "spam")
	call = s.FakeConn.Calls[2]
	c.Check(call.FuncName, gc.Equals, "SetMetadata")
	c.Check(call.InstanceId, gc.Equals, "trucks")
}

func (s *connSuite) TestUpdateMetadataChecksCurrentValue(c *gc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}
	err := s.Conn.UpdateMetadata("eggs", "steak", "spam")
	c.Assert(err, jc.ErrorIsNil)

	// Since the instance already has the right value we don't issue
	// the update.
	c.Assert(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListInstances")
}

func makeMetadataItems(key, value string) *compute.MetadataItems {
	return &compute.MetadataItems{Key: key, Value: google.StringPtr(value)}
}

func checkMetadataItems(c *gc.C, item *compute.MetadataItems, key, value string) {
	c.Assert(item, gc.NotNil)
	c.Check(item.Key, gc.Equals, key)
	c.Assert(item.Value, gc.NotNil)
	c.Check(*item.Value, gc.Equals, value)
}
