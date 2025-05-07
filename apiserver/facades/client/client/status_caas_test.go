// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type CAASStatusSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&CAASStatusSuite{})

func (s *CAASStatusSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Waiting status with "installing agent" info is returned when operator is not ready.
- Status with blocked info is returned when cloud container is set to blocked status.
- Status with workload version set by charm is returned.
`)
}
