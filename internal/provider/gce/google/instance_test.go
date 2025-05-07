// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

type instanceSuite struct {
	google.BaseSuite
}

var _ = tc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *tc.C) {
	inst := google.NewInstanceRaw(&s.RawInstanceFull, &s.InstanceSpec)

	c.Check(inst.ID, tc.Equals, "spam")
	c.Check(inst.ZoneName, tc.Equals, "a-zone")
	c.Check(inst.Status(), tc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), jc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), jc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(inst)
	c.Check(spec, jc.DeepEquals, &s.InstanceSpec)
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

	c.Check(addresses, jc.DeepEquals, s.Addresses)
}

func (s *instanceSuite) TestInstanceMetadata(c *tc.C) {
	metadata := s.Instance.Metadata()

	c.Check(metadata, jc.DeepEquals, map[string]string{"eggs": "steak"})
}

func (s *instanceSuite) TestPackMetadata(c *tc.C) {
	expected := compute.Metadata{Items: []*compute.MetadataItems{
		makeMetadataItems("spam", "eggs"),
	}}
	data := map[string]string{"spam": "eggs"}
	packed := google.PackMetadata(data)

	c.Check(packed, jc.DeepEquals, &expected)
}

func (s *instanceSuite) TestUnpackMetadata(c *tc.C) {
	expected := map[string]string{"spam": "eggs"}
	packed := compute.Metadata{Items: []*compute.MetadataItems{
		makeMetadataItems("spam", "eggs"),
	}}
	data := google.UnpackMetadata(&packed)

	c.Check(data, jc.DeepEquals, expected)
}

func (s *instanceSuite) TestFormatMachineType(c *tc.C) {
	resolved := google.FormatMachineType("a-zone", "spam")

	c.Check(resolved, tc.Equals, "zones/a-zone/machineTypes/spam")
}
