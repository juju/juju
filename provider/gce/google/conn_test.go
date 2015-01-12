// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/errors"
	//	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type connSuite struct {
	google.BaseSuite

	DoCallErr error
}

var _ = gc.Suite(&connSuite{})

func (s *connSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(google.DoCall, func(svc google.Services) (interface{}, error) {
		if svc.ZoneList != nil {
			return s.ZoneList, s.DoCallErr
		}
		return nil, errors.New("no suitable service found")
	})
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
