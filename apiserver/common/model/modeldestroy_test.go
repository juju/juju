// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type destroyModelSuite struct {
	testhelpers.IsolationSuite
}

func TestDestroyModelSuite(t *testing.T) {
	tc.Run(t, &destroyModelSuite{})
}

func (s *destroyModelSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Destroy model
- Destroy model with return error PersistentStorageError
- Destroy controller
- Destroy controller on non controller model
- Destroy controller on non controller model force
- Destroy controller with destroyHostedModels set to true
- Destroy controller with connections to it lingering
- Destroy controller with invalid credential and no force
`)
}
