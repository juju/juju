// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	//"code.google.com/p/google-api-go-client/compute/v1"
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type instanceSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) TestInstanceSpecCreate(c *gc.C) {
	//conn := google.Connection{}
	//zones := []string{"eggs"}
	//s.PatchValue(google.AddInstance, func(c *google.Connection, raw *compute.Instance, typ string, z []string) error {
	//	c.Check(c, gc.Equals, conn)
	//	c.Check(raw.Name, gc.Equals, "spam")
	//	c.Check(raw.Disks, jc.DeepEquals, nil)
	//	c.Check(raw.NetworkInterface, jc.DeepEquals, nil)
	//	c.Check(raw.Metadata, jc.DeepEquals, nil)
	//	c.Check(raw.Tags, jc.DeepEquals, nil)
	//	c.Check(raw.MachineType, gc.Equals, "")
	//	c.Check(typ, gc.Equals, "ham")
	//	c.Check(z, gc.Equals, zones)
	//	return nil
	//})

	//spec := google.InstanceSpec{
	//	ID:    "spam",
	//	Type:  "ham",
	//	Disks: nil,
	//	Network: NetworkSpec{
	//		Name: "default",
	//	},
	//	NetworkInterfaces: nil,
	//	Metadata:          nil,
	//	Tags:              nil,
	//}
	//inst, err := spec.Create(conn, zones)
	//c.Assert(err, jc.ErrorIsNil)

	//c.Check(inst.ID, gc.Equals, "spam")
	//c.Check(inst.Zone, gc.Equals, "eggs")
}

func (s *instanceSuite) TestNewInstance(c *gc.C) {
}

func (s *instanceSuite) TestInstanceRootDiskGB(c *gc.C) {
}

func (s *instanceSuite) TestInstanceStatus(c *gc.C) {
}

func (s *instanceSuite) TestInstanceRefresh(c *gc.C) {
}

func (s *instanceSuite) TestInstanceAddresses(c *gc.C) {
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
