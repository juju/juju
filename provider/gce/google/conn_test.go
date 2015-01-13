// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"regexp"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type connSuite struct {
	google.BaseSuite

	conn         *google.Connection
	op           *compute.Operation
	proj         *compute.Project
	instanceList *compute.InstanceAggregatedList
	firewallList *compute.FirewallList
	zoneList     *compute.ZoneList
	service      *compute.Service

	DoCallErr error
}

var _ = gc.Suite(&connSuite{})

func (s *connSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.service = &compute.Service{BasePath: "localhost"}
	s.service.Zones = compute.NewZonesService(s.service)
	s.service.Projects = compute.NewProjectsService(s.service)
	s.service.ZoneOperations = compute.NewZoneOperationsService(s.service)
	s.service.RegionOperations = compute.NewRegionOperationsService(s.service)
	s.service.GlobalOperations = compute.NewGlobalOperationsService(s.service)
	s.service.Instances = compute.NewInstancesService(s.service)

	s.conn = &google.Connection{
		Region:    "a",
		ProjectID: "spam",
	}
	google.SetRawService(s.conn, s.service)

	s.op = &compute.Operation{}

	s.PatchValue(google.DoCall, s.doCall)
}

func (s *connSuite) TearDownTest(c *gc.C) {
	s.conn = nil
	s.op = nil
	s.proj = nil
	s.instanceList = nil
	s.firewallList = nil
	s.zoneList = nil
	s.service = nil
	s.DoCallErr = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *connSuite) setNoOpWait() {
	s.op.Status = google.StatusDone
	google.SetQuickAttemptStrategy(&s.BaseSuite)
}

func (s *connSuite) TestConnectionConnect(c *gc.C) {
	google.SetRawService(s.conn, nil)
	s.PatchValue(google.NewRawConnection, func(auth google.Auth) (*compute.Service, error) {
		return s.service, nil
	})

	err := s.conn.Connect(s.Auth)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(google.ExposeRawService(s.conn), gc.Equals, s.service)
}

func (s *connSuite) TestConnectionConnectAlreadyConnected(c *gc.C) {
	err := s.conn.Connect(s.Auth)

	c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(`connect() failed (already connected)`))
}

func (s *connSuite) TestConnectionVerifyCredentials(c *gc.C) {
	err := s.conn.VerifyCredentials()

	c.Check(err, jc.ErrorIsNil)
}

func (s *connSuite) TestConnectionVerifyCredentialsInvalid(c *gc.C) {
	s.DoCallErr = errors.New("retrieving auth token for user@mail.com: Invalid Key")
	err := s.conn.VerifyCredentials()

	c.Check(err, gc.ErrorMatches, `retrieving auth token for user@mail.com: Invalid Key`)
}

func (s *connSuite) TestConnectionCheckOperationError(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")
	_, err := google.CheckOperation(s.conn, s.op)

	c.Check(err, gc.ErrorMatches, ".*<unknown>")
}

func (s *connSuite) TestConnectionCheckOperationZone(c *gc.C) {
	original := &compute.Operation{Zone: "a-zone"}
	op, err := google.CheckOperation(s.conn, original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
}

func (s *connSuite) TestConnectionCheckOperationRegion(c *gc.C) {
	original := &compute.Operation{Region: "a"}
	op, err := google.CheckOperation(s.conn, original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
}

func (s *connSuite) TestConnectionCheckOperationGlobal(c *gc.C) {
	original := &compute.Operation{}
	op, err := google.CheckOperation(s.conn, original)

	c.Check(err, jc.ErrorIsNil)
	c.Check(op, gc.NotNil)
	c.Check(op, gc.Not(gc.Equals), original)
}

func (s *connSuite) TestConnectionWaitOperation(c *gc.C) {
	s.op.Status = google.StatusDone
	err := google.WaitOperation(s.conn, s.op, utils.AttemptStrategy{})

	c.Check(err, jc.ErrorIsNil)
}

func (s *connSuite) TestConnectionWaitOperationTimeout(c *gc.C) {
	err := google.WaitOperation(s.conn, s.op, utils.AttemptStrategy{})

	c.Check(err, gc.ErrorMatches, ".* timed out .*")
}

func (s *connSuite) TestConnectionWaitOperationFailure(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")
	err := google.WaitOperation(s.conn, s.op, utils.AttemptStrategy{})

	c.Check(err, gc.ErrorMatches, ".*<unknown>")
}

func (s *connSuite) TestConnectionWaitOperationError(c *gc.C) {
	s.op.Error = &compute.OperationError{}
	s.op.Name = "testing-wait-operation-error"
	err := google.WaitOperation(s.conn, s.op, utils.AttemptStrategy{})

	c.Check(err, gc.ErrorMatches, `.* "testing-wait-operation-error" .*`)
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

	azs, err := s.conn.AvailabilityZones("a-zone")
	c.Check(err, gc.IsNil)
	c.Check(len(azs), gc.Equals, 1)
}

func (s *connSuite) TestConnectionAvailabilityZonesErr(c *gc.C) {
	s.DoCallErr = errors.New("<unknown>")

	azs, err := s.conn.AvailabilityZones("a-zone")
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
		return &s.RawInstance, s.DoCallErr
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
