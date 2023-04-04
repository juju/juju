// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
)

type SecretBackendSuite struct{}

var _ = gc.Suite(&SecretBackendSuite{})

func (s *SecretBackendSuite) TestNextBackendRotateTimeTooShort(c *gc.C) {
	_, err := secrets.NextBackendRotateTime(time.Now(), time.Minute)
	c.Assert(err, gc.ErrorMatches, `token rotate interval "1m0s" less than 1h not valid`)
}

func (s *SecretBackendSuite) TestNextBackendRotateTime(c *gc.C) {
	now := time.Now()
	next, err := secrets.NextBackendRotateTime(now, 200*time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next.Sub(now), gc.Equals, 150*time.Minute)
}

func (s *SecretBackendSuite) TestNextBackendRotateTimeMax(c *gc.C) {
	now := time.Now()
	next, err := secrets.NextBackendRotateTime(now, 60*24*time.Hour)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(next.Sub(now), gc.Equals, 24*time.Hour)
}
