// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type connSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&connSuite{})

func (s *connSuite) TestConnectionConnect(c *gc.C) {
}

func (s *connSuite) TestConnectionVerifyCredentials(c *gc.C) {
}

func (s *connSuite) TestConnectionCheckOperation(c *gc.C) {
}

func (s *connSuite) TestConnectionWaitOperation(c *gc.C) {
}

func (s *connSuite) TestConnectionAvailabilityZones(c *gc.C) {
}
