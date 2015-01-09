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
	s.PatchValue(google.AddInstance, func(conn *google.Connection, raw *compute.Instance, typ string, zones []string) error {
		s.RawInstance.Zone = zones[0]
		*raw = s.RawInstance

		return nil
	})

	conn := google.Connection{}
	zones := []string{"eggs"}

	inst, err := s.InstanceSpec.Create(&conn, zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.Zone, gc.Equals, "eggs")
	c.Check(inst.Spec(), gc.DeepEquals, &s.InstanceSpec)
	c.Check(google.ExposeRawInstance(inst), gc.DeepEquals, &s.RawInstance)
}

func (s *instanceSuite) TestInstanceSpecCreateAPI(c *gc.C) {
	var connArg *google.Connection
	var rawArg compute.Instance
	var typeArg string
	var zonesArg []string

	s.PatchValue(google.AddInstance, func(conn *google.Connection, raw *compute.Instance, typ string, zones []string) error {
		connArg = conn
		rawArg = *raw
		typeArg = typ
		zonesArg = zones
		*raw = s.RawInstance

		return nil
	})

	conn := google.Connection{}
	zones := []string{"eggs"}
	_, err := s.InstanceSpec.Create(&conn, zones)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(connArg, gc.Equals, &conn)
	c.Check(typeArg, gc.Equals, "sometype")
	c.Check(zonesArg, jc.DeepEquals, zones)

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

	c.Check(rawArg, jc.DeepEquals, compute.Instance{
		Name:              "spam",
		Disks:             attachedDisks,
		NetworkInterfaces: networkInterfaces,
		Metadata:          &metadata,
		Tags:              &compute.Tags{Items: []string{"spam"}},
	})
}

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := google.NewInstance(&s.RawInstance)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.Zone, gc.Equals, "a-zone")
	c.Check(google.ExposeRawInstance(inst), gc.DeepEquals, &s.RawInstance)
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

	c.Check(status, gc.Equals, "UP")
}

func (s *instanceSuite) TestInstanceStatusDown(c *gc.C) {
	google.ExposeRawInstance(&s.Instance).Status = google.StatusDown
	status := s.Instance.Status()

	c.Check(status, gc.Equals, "DOWN")
}

func (s *instanceSuite) TestInstanceRefresh(c *gc.C) {
	// TODO(wwitzel3) add test after we finish conn_* suite of tests
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
}

func (s *instanceSuite) TestFilterInstances(c *gc.C) {
}

func (s *instanceSuite) TestCheckInstStatus(c *gc.C) {
}

func (s *instanceSuite) TestFormatAuthorizedKeys(c *gc.C) {
}

func (s *instanceSuite) TestPackMetadata(c *gc.C) {
}

func (s *instanceSuite) TestUnpackMetadata(c *gc.C) {
}

func (s *instanceSuite) TestResolveMachineType(c *gc.C) {
}
