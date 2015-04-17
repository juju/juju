// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type instanceSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestNewInstance(c *gc.C) {
	inst := google.NewInstanceRaw(&s.RawInstanceFull, &s.InstanceSpec)

	c.Check(inst.ID, gc.Equals, "spam")
	c.Check(inst.ZoneName, gc.Equals, "a-zone")
	c.Check(inst.Status(), gc.Equals, google.StatusRunning)
	c.Check(inst.Metadata(), jc.DeepEquals, s.Metadata)
	c.Check(inst.Addresses(), jc.DeepEquals, s.Addresses)
	spec := google.GetInstanceSpec(inst)
	c.Check(spec, jc.DeepEquals, &s.InstanceSpec)
}

func (s *instanceSuite) TestNewInstanceNoSpec(c *gc.C) {
	inst := google.NewInstanceRaw(&s.RawInstanceFull, nil)

	spec := google.GetInstanceSpec(inst)
	c.Check(spec, gc.IsNil)
}

func (s *instanceSuite) TestInstanceRootDiskGB(c *gc.C) {
	size := s.Instance.RootDiskGB()

	c.Check(size, gc.Equals, uint64(15))
}

func (s *instanceSuite) TestInstanceRootDiskGBNilSpec(c *gc.C) {
	inst := google.Instance{}
	size := inst.RootDiskGB()

	c.Check(size, gc.Equals, uint64(0))
}

func (s *instanceSuite) TestInstanceStatus(c *gc.C) {
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusRunning)
}

func (s *instanceSuite) TestInstanceStatusDown(c *gc.C) {
	s.Instance.InstanceSummary.Status = google.StatusDown
	status := s.Instance.Status()

	c.Check(status, gc.Equals, google.StatusDown)
}

func (s *instanceSuite) TestInstanceRefresh(c *gc.C) {
	s.FakeConn.Instance = &s.RawInstanceFull

	specBefore := google.GetInstanceSpec(&s.Instance)
	err := s.Instance.Refresh(s.Conn)
	c.Assert(err, jc.ErrorIsNil)
	specAfter := google.GetInstanceSpec(&s.Instance)

	c.Check(s.Instance.ID, gc.Equals, "spam")
	c.Check(s.Instance.ZoneName, gc.Equals, "a-zone")
	c.Check(s.Instance.Status(), gc.Equals, google.StatusRunning)
	c.Check(s.Instance.Metadata(), jc.DeepEquals, s.Metadata)
	c.Check(s.Instance.Addresses(), jc.DeepEquals, s.Addresses)
	c.Check(specAfter, gc.Equals, specBefore)
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

	c.Check(addresses, jc.DeepEquals, s.Addresses)
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

	c.Check(err, gc.ErrorMatches, "empty rawAuthorizedKeys")
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

func (s *instanceSuite) TestFormatMachineType(c *gc.C) {
	resolved := google.FormatMachineType("a-zone", "spam")

	c.Check(resolved, gc.Equals, "zones/a-zone/machineTypes/spam")
}
