// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type CAASStatusSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CAASStatusSuite{})

func (s *CAASStatusSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Waiting status with "installing agent" info is returned when operator is not ready.
- Status with blocked info is returned when cloud container is set to blocked status.
- Status with workload version set by charm is returned.
`)
}
