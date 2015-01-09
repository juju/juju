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

	c.Check(status, gc.Equals, google.StatusRunning)
}

func (s *instanceSuite) TestInstanceStatusDown(c *gc.C) {
	google.ExposeRawInstance(&s.Instance).Status = google.StatusDown
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusDown)
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
	metadata := s.Instance.Metadata()

	c.Check(metadata, jc.DeepEquals, map[string]string{"eggs": "steak"})
}

func (s *instanceSuite) TestFilterInstances(c *gc.C) {
	instances := []google.Instance{s.Instance}
	matched := google.FilterInstances(instances, google.StatusRunning)

	c.Check(matched, jc.DeepEquals, instances)
}

func (s *instanceSuite) TestFilterInstancesEmpty(c *gc.C) {
	matched := google.FilterInstances(nil, google.StatusRunning)

	c.Check(matched, gc.HasLen, 0)
}

func (s *instanceSuite) TestFilterInstancesNoStatus(c *gc.C) {
	instances := []google.Instance{s.Instance}
	matched := google.FilterInstances(instances)

	c.Check(matched, gc.HasLen, 0)
}

func (s *instanceSuite) TestFilterInstancesNoMatch(c *gc.C) {
	instances := []google.Instance{s.Instance}
	matched := google.FilterInstances(instances, google.StatusDown)

	c.Check(matched, gc.HasLen, 0)
}

func (s *instanceSuite) TestFilterInstancesMixedStatus(c *gc.C) {
	badInst := google.NewInstance(&compute.Instance{
		Status: google.StatusDown,
	})
	instances := []google.Instance{
		s.Instance,
		*badInst,
	}
	matched := google.FilterInstances(instances, google.StatusRunning)

	c.Check(matched, jc.DeepEquals, []google.Instance{s.Instance})
}

func (s *instanceSuite) TestFilterInstancesMultiStatus(c *gc.C) {
	otherInst := google.NewInstance(&compute.Instance{
		Status: google.StatusPending,
	})
	badInst := google.NewInstance(&compute.Instance{
		Status: google.StatusDown,
	})
	instances := []google.Instance{
		s.Instance,
		*otherInst,
		*badInst,
	}
	matched := google.FilterInstances(instances, google.StatusRunning, google.StatusPending)

	c.Check(matched, jc.DeepEquals, []google.Instance{s.Instance, *otherInst})
}

func (s *instanceSuite) TestCheckInstStatus(c *gc.C) {
	matched := google.CheckInstStatus(s.Instance, google.StatusRunning)

	c.Check(matched, jc.IsTrue)
}

func (s *instanceSuite) TestCheckInstStatusNoMatch(c *gc.C) {
	matched := google.CheckInstStatus(s.Instance, google.StatusPending)

	c.Check(matched, jc.IsFalse)
}

func (s *instanceSuite) TestCheckInstStatusNoStatus(c *gc.C) {
	matched := google.CheckInstStatus(s.Instance)

	c.Check(matched, jc.IsFalse)
}

func (s *instanceSuite) TestCheckInstStatusMultiStatus(c *gc.C) {
	matched := google.CheckInstStatus(s.Instance, google.StatusRunning, google.StatusPending)

	c.Check(matched, jc.IsTrue)
}

func (s *instanceSuite) TestFormatAuthorizedKeys(c *gc.C) {
	formatted, err := google.FormatAuthorizedKeys("abcd", "john")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(formatted, gc.Equals, "john:abcd\n")
}

func (s *instanceSuite) TestFormatAuthorizedKeysEmpty(c *gc.C) {
	formatted, err := google.FormatAuthorizedKeys("", "john")
	c.Assert(err, jc.ErrorIsNil)

	// XXX Is this right?
	c.Check(formatted, gc.Equals, "john:\n")
}

func (s *instanceSuite) TestFormatAuthorizedKeysNoUser(c *gc.C) {
	formatted, err := google.FormatAuthorizedKeys("abcd", "")
	c.Assert(err, jc.ErrorIsNil)

	// XXX Is this right?
	c.Check(formatted, gc.Equals, ":abcd\n")
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
