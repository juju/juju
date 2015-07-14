// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type environ struct {
	suite *JujuConnSuite
}

// NewEnv returns a new implementation of coretesting.Environ that
// wraps the dummy provider used by JujuConnSuite.
func NewEnv(s *JujuConnSuite) coretesting.Environ {
	return &environ{suite: s}
}

// AddService implements coretesting.Environ.
func (env *environ) AddService(c *gc.C, charmName, serviceName string) coretesting.Service {
	ch := env.suite.AddTestingCharm(c, charmName)
	svc := env.suite.AddTestingService(c, serviceName, ch)

	return &service{
		env:     env,
		charm:   ch,
		service: svc,
	}
}

// Destroy implements coretesting.Environ.
func (*environ) Destroy(c *gc.C) {
	// no-op
}
