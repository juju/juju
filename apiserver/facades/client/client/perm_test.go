// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

// Most (if not all) of the permission tests below aim to test
// end-to-end operations execution through the API, but do not care
// about the results. They only test that a call is succeeds or fails
// (usually due to "permission denied"). There are separate test cases
// testing each individual API call data flow later on.
type permSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&permSuite{})

type watcherPermSuite struct {
	permSuite
}

var _ = tc.Suite(&watcherPermSuite{})

func (s *permSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Not found should be returned when trying to add a relation between non-exsting endpoints.
- Not found should be returned when trying to destroy a relation between non-exsting endpoints.
- A correct status is returned for a pre-filled model with a scenario.
- A correct application is returned for a pre-filled model with a scenario.
- No error is returned when trying to expose endpoints for an existing application.
- No error is returned when trying to unexpose endpoints for an existing application.
- Unathorized error is returned when trying to resolve units using an unathorized user.
- No error is returned when trying to resolve units.
- A correct set of annotations is returned for an existing application.
- No errors are returned when trying to set annotations for an existing application.
- No errors are returned when trying to set charm for an existing application.
- No errors are returned when trying to add units for an existing application.
- No errors are returned when trying to destroy units for an existing application.
- No errors are returned when trying to destroy an application.
- Correct constraints are returned for an existing application.
- No errors are returned when trying to set constraints for an existing application.
- No errors are returned when trying to set model constraints.
- No errors are returned when trying to get a model.
- No errors are returned when trying to set the model config.
`)
}
