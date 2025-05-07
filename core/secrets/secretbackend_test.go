// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/secrets"
)

type SecretBackendSuite struct{}

var _ = tc.Suite(&SecretBackendSuite{})

func (s *SecretBackendSuite) TestNextBackendRotateTimeTooShort(c *tc.C) {
	_, err := secrets.NextBackendRotateTime(time.Now(), time.Minute)
	c.Assert(err, tc.ErrorMatches, `token rotate interval "1m0s" less than 1h not valid`)
}

func (s *SecretBackendSuite) TestNextBackendRotateTime(c *tc.C) {
	now := time.Now()
	next, err := secrets.NextBackendRotateTime(now, 200*time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next.Sub(now), tc.Equals, 150*time.Minute)
}

func (s *SecretBackendSuite) TestNextBackendRotateTimeMax(c *tc.C) {
	now := time.Now()
	next, err := secrets.NextBackendRotateTime(now, 60*24*time.Hour)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next.Sub(now), tc.Equals, 24*time.Hour)
}
