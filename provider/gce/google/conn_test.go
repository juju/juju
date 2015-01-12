// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	//	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type connSuite struct {
	google.BaseSuite

	op           *compute.Operation
	proj         *compute.Project
	instanceList *compute.InstanceList
	firewallList *compute.FirewallList
	zoneList     *compute.ZoneList

	DoCallErr error
}

var _ = gc.Suite(&connSuite{})

func (s *connSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.op = &compute.Operation{}

	s.PatchValue(google.DoCall, s.doCall)
}
func (s *connSuite) TestConnectionConnect(c *gc.C) {
}

func (s *connSuite) TestConnectionVerifyCredentials(c *gc.C) {
}

func (s *connSuite) TestConnectionCheckOperation(c *gc.C) {
}

func (s *connSuite) TestConnectionWaitOperation(c *gc.C) {
}

func (s *connSuite) TestConnectionAvailabilityZones(c *gc.C) {
	s.zoneList = &compute.ZoneList{
		Id: "testing-zone-list",
		Items: []*compute.Zone{{
			Name:   "a-zone",
			Status: "UP",
		}},
		NextPageToken: "",
	}

	azs, err := s.Conn.AvailabilityZones("a-zone")
	c.Check(err, gc.IsNil)
	c.Check(len(azs), gc.Equals, 1)
}

func (s *connSuite) TestConnectionAvailabilityZonesErr(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")

	azs, err := s.Conn.AvailabilityZones("a-zone")
	c.Check(err, gc.ErrorMatches, "<unknown>")
	c.Check(len(azs), gc.Equals, 0)
}

func (s *connSuite) doCall(svc google.Services) (interface{}, error) {
	switch {
	case svc.ZoneList != nil:
		return s.zoneList, s.DoCallErr
	case svc.ZoneOp != nil:
		return s.op, s.DoCallErr
	case svc.RegionOp != nil:
		return s.op, s.DoCallErr
	case svc.GlobalOp != nil:
		return s.op, s.DoCallErr
	case svc.InstanceList != nil:
		return s.instanceList, s.DoCallErr
	case svc.InstanceGet != nil:
		return s.RawInstance, s.DoCallErr
	case svc.InstanceInsert != nil:
		return s.op, s.DoCallErr
	case svc.InstanceDelete != nil:
		return s.op, s.DoCallErr
	case svc.ProjectGet != nil:
		return s.proj, s.DoCallErr
	case svc.FirewallList != nil:
		return s.firewallList, s.DoCallErr
	case svc.FirewallInsert != nil:
		return s.op, s.DoCallErr
	case svc.FirewallUpdate != nil:
		return s.op, s.DoCallErr
	case svc.FirewallDelete != nil:
		return s.op, s.DoCallErr
	}
	return nil, errors.New("no suitable service found")
}
