// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/gce/google"
)

type connSuite struct {
	google.BaseSuite
}

func TestConnSuite(t *stdtesting.T) {
	tc.Run(t, &connSuite{})
}

func (s *connSuite) TestConnect(c *tc.C) {
	google.SetRawConn(s.Conn, nil)
	service := &compute.Service{}
	s.PatchValue(google.NewService, func(ctx context.Context, creds *google.Credentials, httpClient *jujuhttp.Client) (*compute.Service, error) {
		return service, nil
	})

	conn, err := google.Connect(c.Context(), s.ConnCfg, s.Credentials)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(google.ExposeRawService(conn), tc.Equals, service)
}

func (s *connSuite) TestConnectionVerifyCredentials(c *tc.C) {
	s.FakeConn.Project = &compute.Project{}
	err := s.Conn.VerifyCredentials()

	c.Check(err, tc.ErrorIsNil)
}

func (s *connSuite) TestConnectionVerifyCredentialsAPI(c *tc.C) {
	s.FakeConn.Project = &compute.Project{}
	err := s.Conn.VerifyCredentials()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetProject")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
}

func (s *connSuite) TestConnectionVerifyCredentialsInvalid(c *tc.C) {
	s.FakeConn.Err = errors.New("retrieving auth token for user@mail.com: Invalid Key")
	err := s.Conn.VerifyCredentials()

	c.Check(err, tc.ErrorMatches, `retrieving auth token for user@mail.com: Invalid Key`)
}

func (s *connSuite) TestConnectionAvailabilityZones(c *tc.C) {
	s.FakeConn.Zones = []*compute.Zone{{
		Name:   "a-zone",
		Status: google.StatusUp,
	}}

	azs, err := s.Conn.AvailabilityZones("a")
	c.Check(err, tc.IsNil)

	c.Check(len(azs), tc.Equals, 1)
	c.Check(azs[0].Name(), tc.Equals, "a-zone")
	c.Check(azs[0].Status(), tc.Equals, google.StatusUp)
}

func (s *connSuite) TestConnectionAvailabilityZonesAPI(c *tc.C) {
	_, err := s.Conn.AvailabilityZones("a")
	c.Assert(err, tc.IsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListAvailabilityZones")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].Region, tc.Equals, "a")
}

func (s *connSuite) TestConnectionAvailabilityZonesErr(c *tc.C) {
	s.FakeConn.Err = errors.New("<unknown>")

	_, err := s.Conn.AvailabilityZones("a")

	c.Check(err, tc.ErrorMatches, "<unknown>")
}
