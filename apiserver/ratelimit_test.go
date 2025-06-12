// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"testing"

	"github.com/juju/tc"
)

type rateLimitSuite struct {
}

func TestRateLimitSuite(t *testing.T) {
	tc.Run(t, &rateLimitSuite{})
}

func (s *rateLimitSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:
- Rate limit for machines is not applied when the controller is not configured.
- Rate limit for machines is applied when the controller is configured.
- Rate limit for machines is not applied when the controller is configured, but the rate limit is set to 0.
`)
}
