// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

func (s *connSuite) TestConnectionSimpleAddInstance(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst := &s.RawInstance
	err := google.ConnAddInstance(s.Conn, inst, "mtype", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(inst, tc.DeepEquals, &s.RawInstanceFull)
}

func (s *connSuite) TestConnectionSimpleAddInstanceAPI(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull
	expected := s.RawInstance
	expected.MachineType = "zones/a-zone/machineTypes/mtype"

	inst := &s.RawInstance
	err := google.ConnAddInstance(s.Conn, inst, "mtype", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "AddInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[0].InstValue, tc.DeepEquals, expected)
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[1].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, tc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[1].ID, tc.Equals, "spam")
}

func (s *instanceSuite) TestConnectionAddInstance(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst, err := s.Conn.AddInstance(s.InstanceSpec)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(inst.ID, tc.Equals, "spam")
	c.Check(inst.ZoneName, tc.Equals, "a-zone")
	c.Check(inst.Status(), tc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), tc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), tc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(inst)
	c.Check(spec, tc.DeepEquals, &s.InstanceSpec)
}

func (s *instanceSuite) TestConnectionAddInstanceAPI(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	_, err := s.Conn.AddInstance(s.InstanceSpec)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "AddInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	// We check s.FakeConn.Calls[0].InstValue below.
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[1].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, tc.Equals, "a-zone")

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
	c.Check(s.FakeConn.Calls[0].InstValue, tc.DeepEquals, compute.Instance{
		Name:              "spam",
		MachineType:       "zones/a-zone/machineTypes/mtype",
		Disks:             attachedDisks,
		NetworkInterfaces: networkInterfaces,
		Metadata:          &metadata,
		Tags:              &compute.Tags{Items: []string{"spam"}},
	})
}

func (s *connSuite) TestConnectionAddInstanceFailed(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *connSuite) TestConnectionAddInstanceWaitFailed(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	cause := errors.New("unknown")
	failure := s.NewWaitError(nil, cause)
	s.FakeConn.Err = failure

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, cause)
}

func (s *connSuite) TestConnectionAddInstanceGetFailed(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	failure := errors.New("unknown")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 1

	err := google.ConnAddInstance(s.Conn, &s.RawInstance, "mtype", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
}

func (s *connSuite) TestConnectionInstance(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	inst, err := s.Conn.Instance("spam", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(inst.ID, tc.Equals, "spam")
	c.Check(inst.ZoneName, tc.Equals, "a-zone")
	c.Check(inst.Status(), tc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), tc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), tc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(&inst)
	c.Check(spec, tc.IsNil)
}

func (s *connSuite) TestConnectionInstanceAPI(c *tc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	_, err := s.Conn.Instance("ham", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ID, tc.Equals, "ham")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "a-zone")
}

func (s *connSuite) TestConnectionInstanceFail(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	_, err := s.Conn.Instance("spam", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *connSuite) TestConnectionInstances(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	insts, err := s.Conn.Instances("sp", google.StatusRunning)
	c.Assert(err, tc.ErrorIsNil)

	google.SetInstanceSpec(&s.Instance, nil)
	c.Check(insts, tc.DeepEquals, []google.Instance{s.Instance})
}

func (s *connSuite) TestConnectionInstancesFailure(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	_, err := s.Conn.Instances("sp", google.StatusRunning)

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstance(c *tc.C) {
	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(err, tc.ErrorIsNil)
}

func (s *connSuite) TestConnectionRemoveInstanceAPI(c *tc.C) {
	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[0].ID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[1].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].Name, tc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstanceFailed(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstanceFirewallFailed(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 1

	err := google.ConnRemoveInstance(s.Conn, "spam", "a-zone")

	c.Check(errors.Cause(err), tc.Equals, failure)
	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
}

func (s *connSuite) TestConnectionRemoveInstances(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(err, tc.ErrorIsNil)
}

func (s *connSuite) TestConnectionRemoveInstancesAPI(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.RemoveInstances("sp", "spam")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, tc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstancesMultiple(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{
		&s.RawInstanceFull,
		{
			Name: "special",
			Zone: "a-zone",
		},
	}

	err := s.Conn.RemoveInstances("", "spam", "special")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 5)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[3].FuncName, tc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[3].ID, tc.Equals, "special")
	c.Check(s.FakeConn.Calls[4].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[4].Name, tc.Equals, "special")
}

func (s *connSuite) TestConnectionRemoveInstancesPartialMatch(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{
		&s.RawInstanceFull,
		{
			Name: "special",
			Zone: "a-zone",
		},
	}

	err := s.Conn.RemoveInstances("", "spam")
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")
	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "RemoveInstance")
	c.Check(s.FakeConn.Calls[1].ID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[2].FuncName, tc.Equals, "RemoveFirewall")
	c.Check(s.FakeConn.Calls[2].Name, tc.Equals, "spam")
}

func (s *connSuite) TestConnectionRemoveInstancesListFailed(c *tc.C) {
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(errors.Cause(err), tc.Equals, failure)
}

func (s *connSuite) TestConnectionRemoveInstancesRemoveFailed(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}
	failure := errors.New("<unknown>")
	s.FakeConn.Err = failure
	s.FakeConn.FailOnCall = 2

	err := s.Conn.RemoveInstances("sp", "spam")

	c.Check(err, tc.ErrorMatches, ".*some instance removals failed: .*")
}

func (s *connSuite) TestUpdateMetadataNewAttribute(c *tc.C) {
	// Ensure we extract the name from the URL we get on the raw instance.
	s.RawInstanceFull.Zone = "http://eels/lone/wolf/a-zone"
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.UpdateMetadata("business", "time", s.RawInstanceFull.Name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.ProjectID, tc.Equals, "spam")
	c.Check(call.ZoneName, tc.Equals, "a-zone")
	c.Check(call.InstanceId, tc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, tc.Equals, "heymumwatchthis")
	c.Assert(md.Items, tc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "steak")
	checkMetadataItems(c, md.Items[1], "business", "time")
}

func (s *connSuite) TestUpdateMetadataExistingAttribute(c *tc.C) {
	// Ensure we extract the name from the URL we get on the raw instance.
	s.RawInstanceFull.Zone = "http://eels/lone/wolf/a-zone"
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}

	err := s.Conn.UpdateMetadata("eggs", "beans", s.RawInstanceFull.Name)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.ProjectID, tc.Equals, "spam")
	c.Check(call.ZoneName, tc.Equals, "a-zone")
	c.Check(call.InstanceId, tc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, tc.Equals, "heymumwatchthis")
	c.Assert(md.Items, tc.HasLen, 1)
	checkMetadataItems(c, md.Items[0], "eggs", "beans")
}

func (s *connSuite) TestUpdateMetadataMultipleInstances(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.ProjectID, tc.Equals, "spam")
	c.Check(call.ZoneName, tc.Equals, "a-zone")
	c.Check(call.InstanceId, tc.Equals, "spam")

	md := call.Metadata
	c.Check(md.Fingerprint, tc.Equals, "heymumwatchthis")
	c.Assert(md.Items, tc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "steak")
	checkMetadataItems(c, md.Items[1], "rick", "morty")

	call = s.FakeConn.Calls[2]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.ProjectID, tc.Equals, "spam")
	c.Check(call.ZoneName, tc.Equals, "a-zone")
	c.Check(call.InstanceId, tc.Equals, "boats")

	md = call.Metadata
	c.Check(md.Fingerprint, tc.Equals, "imprisoned")
	c.Assert(md.Items, tc.HasLen, 2)
	checkMetadataItems(c, md.Items[0], "eggs", "milk")
	checkMetadataItems(c, md.Items[1], "rick", "morty")
}

func (s *connSuite) TestUpdateMetadataError(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, `some metadata updates failed: \[spam\]`)

	c.Assert(s.FakeConn.Calls, tc.HasLen, 3)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")

	call := s.FakeConn.Calls[1]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.InstanceId, tc.Equals, "spam")
	call = s.FakeConn.Calls[2]
	c.Check(call.FuncName, tc.Equals, "SetMetadata")
	c.Check(call.InstanceId, tc.Equals, "trucks")
}

func (s *connSuite) TestUpdateMetadataChecksCurrentValue(c *tc.C) {
	s.FakeConn.Instances = []*compute.Instance{&s.RawInstanceFull}
	err := s.Conn.UpdateMetadata("eggs", "steak", "spam")
	c.Assert(err, tc.ErrorIsNil)

	// Since the instance already has the right value we don't issue
	// the update.
	c.Assert(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListInstances")
}

func makeMetadataItems(key, value string) *compute.MetadataItems {
	return &compute.MetadataItems{Key: key, Value: google.StringPtr(value)}
}

func checkMetadataItems(c *tc.C, item *compute.MetadataItems, key, value string) {
	c.Assert(item, tc.NotNil)
	c.Check(item.Key, tc.Equals, key)
	c.Assert(item.Value, tc.NotNil)
	c.Check(*item.Value, tc.Equals, value)
}
