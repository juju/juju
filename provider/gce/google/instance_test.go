// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/gce/google"
)

type instanceSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestInstanceSpecCreate(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	zones := []string{"a-zone"}
	inst, err := s.InstanceSpec.Create(s.Conn, zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.Zone, gc.Equals, "a-zone")
	c.Check(inst.Spec(), gc.DeepEquals, &s.InstanceSpec)
	c.Check(google.ExposeRawInstance(inst), gc.DeepEquals, &s.RawInstanceFull)
}

func (s *instanceSuite) TestInstanceSpecCreateAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	zones := []string{"a-zone"}
	_, err := s.InstanceSpec.Create(s.Conn, zones)
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
			DiskSizeGb:  1,
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

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := google.NewInstance(&s.RawInstanceFull)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.Zone, gc.Equals, "a-zone")
	c.Check(google.ExposeRawInstance(inst), gc.DeepEquals, &s.RawInstanceFull)
}

func (s *instanceSuite) TestInstanceSpec(c *gc.C) {
	spec := s.Instance.Spec()

	c.Check(spec, gc.Equals, &s.InstanceSpec)
}

func (s *instanceSuite) TestInstanceRootDiskGB(c *gc.C) {
	size := s.Instance.RootDiskGB()

	c.Check(size, gc.Equals, int64(1))
}

func (s *instanceSuite) TestInstanceRootDiskGBNilSpec(c *gc.C) {
	inst := google.Instance{}
	size := inst.RootDiskGB()

	c.Check(size, gc.Equals, int64(0))
}

func (s *instanceSuite) TestInstanceStatus(c *gc.C) {
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusRunning)
}

func (s *instanceSuite) TestInstanceStatusDown(c *gc.C) {
	google.ExposeRawInstance(&s.Instance).Status = google.StatusDown
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusDown)
}

func (s *instanceSuite) TestInstanceRefresh(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull
	google.SetRawInstance(&s.Instance, compute.Instance{})

	err := s.Instance.Refresh(s.Conn)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(google.ExposeRawInstance(&s.Instance), jc.DeepEquals, &s.RawInstanceFull)
}

func (s *instanceSuite) TestInstanceRefreshAPI(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	err := s.Instance.Refresh(s.Conn)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetInstance")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "a-zone")
	c.Check(s.FakeConn.Calls[0].ID, gc.Equals, "spam")
}

func (s *instanceSuite) TestInstanceAddresses(c *gc.C) {
	addresses := s.Instance.Addresses()

	c.Check(addresses, jc.DeepEquals, []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}})
}

func (s *instanceSuite) TestInstanceAddressesExternal(c *gc.C) {
	s.NetworkInterface.NetworkIP = ""
	s.NetworkInterface.AccessConfigs[0].NatIP = "8.8.8.8"
	addresses := s.Instance.Addresses()

	c.Check(addresses, jc.DeepEquals, []network.Address{{
		Value: "8.8.8.8",
		Type:  network.IPv4Address,
		Scope: network.ScopePublic,
	}})
}

func (s *instanceSuite) TestInstanceAddressesEmpty(c *gc.C) {
	s.NetworkInterface.AccessConfigs = nil
	s.NetworkInterface.NetworkIP = ""
	addresses := s.Instance.Addresses()

	c.Check(addresses, gc.HasLen, 0)
}

func (s *instanceSuite) TestInstanceMetadata(c *gc.C) {
	metadata := s.Instance.Metadata()

	c.Check(metadata, jc.DeepEquals, map[string]string{"eggs": "steak"})
}

func (s *instanceSuite) TestFormatAuthorizedKeys(c *gc.C) {
	formatted, err := google.FormatAuthorizedKeys("abcd", "john")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, gc.Equals, "john:abcd\n")
}

func (s *instanceSuite) TestFormatAuthorizedKeysEmpty(c *gc.C) {
	_, err := google.FormatAuthorizedKeys("", "john")

	c.Check(err, gc.ErrorMatches, "empty raw")
}

func (s *instanceSuite) TestFormatAuthorizedKeysNoUser(c *gc.C) {
	_, err := google.FormatAuthorizedKeys("abcd", "")

	c.Check(err, gc.ErrorMatches, "empty user")
}

func (s *instanceSuite) TestFormatAuthorizedKeysMultiple(c *gc.C) {
	formatted, err := google.FormatAuthorizedKeys("abcd\ndcba\nqwer", "john")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, gc.Equals, "john:abcd\njohn:dcba\njohn:qwer\n")
}

func (s *instanceSuite) TestPackMetadata(c *gc.C) {
	expected := compute.Metadata{Items: []*compute.MetadataItems{{
		Key:   "spam",
		Value: "eggs",
	}}}
	data := map[string]string{"spam": "eggs"}
	packed := google.PackMetadata(data)

	c.Check(packed, jc.DeepEquals, &expected)
}

func (s *instanceSuite) TestUnpackMetadata(c *gc.C) {
	expected := map[string]string{"spam": "eggs"}
	packed := compute.Metadata{Items: []*compute.MetadataItems{{
		Key:   "spam",
		Value: "eggs",
	}}}
	data := google.UnpackMetadata(&packed)

	c.Check(data, jc.DeepEquals, expected)
}

func (s *instanceSuite) TestResolveMachineType(c *gc.C) {
	resolved := google.ResolveMachineType("a-zone", "spam")

	c.Check(resolved, gc.Equals, "zones/a-zone/machineTypes/spam")
}
