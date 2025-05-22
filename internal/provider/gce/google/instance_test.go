// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"testing"

	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

type instanceSuite struct {
	google.BaseSuite
}

func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	inst := google.NewInstanceRaw(&s.RawInstanceFull, &s.InstanceSpec)

	c.Check(inst.ID, tc.Equals, "spam")
	c.Check(inst.ZoneName, tc.Equals, "a-zone")
	c.Check(inst.Status(), tc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), tc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), tc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(inst)
	c.Check(spec, tc.DeepEquals, &s.InstanceSpec)
}

func (s *instanceSuite) TestNewInstanceNoSpec(c *tc.C) {
	inst := google.NewInstanceRaw(&s.RawInstanceFull, nil)

	spec := google.GetInstanceSpec(inst)
	c.Check(spec, tc.IsNil)
}

func (s *instanceSuite) TestInstanceRootDiskGB(c *tc.C) {
	size := s.Instance.RootDiskGB()

	c.Check(size, tc.Equals, uint64(15))
}

func (s *instanceSuite) TestInstanceRootDiskGBNilSpec(c *tc.C) {
	inst := google.Instance{}
	size := inst.RootDiskGB()

	c.Check(size, tc.Equals, uint64(0))
}

func (s *instanceSuite) TestInstanceStatus(c *tc.C) {
	status := s.Instance.Status()

	c.Check(status, tc.Equals, google.StatusRunning)
}

func (s *instanceSuite) TestInstanceStatusDown(c *tc.C) {
	s.Instance.InstanceSummary.Status = google.StatusDown
	status := s.Instance.Status()

	c.Check(status, tc.Equals, google.StatusDown)
}

func (s *instanceSuite) TestInstanceAddresses(c *tc.C) {
	addresses := s.Instance.Addresses()

	c.Check(addresses, tc.DeepEquals, s.Addresses)
}

func (s *instanceSuite) TestInstanceMetadata(c *tc.C) {
	metadata := s.Instance.Metadata()

	c.Check(metadata, tc.DeepEquals, map[string]string{"eggs": "steak"})
}

func (s *instanceSuite) TestPackMetadata(c *tc.C) {
	expected := compute.Metadata{Items: []*compute.MetadataItems{
		makeMetadataItems("spam", "eggs"),
	}}
	data := map[string]string{"spam": "eggs"}
	packed := google.PackMetadata(data)

	c.Check(packed, tc.DeepEquals, &expected)
}

func (s *instanceSuite) TestUnpackMetadata(c *tc.C) {
	expected := map[string]string{"spam": "eggs"}
	packed := compute.Metadata{Items: []*compute.MetadataItems{
		makeMetadataItems("spam", "eggs"),
	}}
	data := google.UnpackMetadata(&packed)

	c.Check(data, tc.DeepEquals, expected)
}

func (s *instanceSuite) TestFormatMachineType(c *tc.C) {
	resolved := google.FormatMachineType("a-zone", "spam")

	c.Check(resolved, tc.Equals, "zones/a-zone/machineTypes/spam")
}
