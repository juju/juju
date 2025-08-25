// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type statusSuite struct {
	testhelpers.IsolationSuite
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &statusSuite{})
}

func (s *statusSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Full status (assert on machine, application, unit, relation, model).
- Full status with leadership (assert a unit is leader).
- Full status before and after a unit has been scaled.
- Full status before and after a machine has been scaled.
- Full status before and after NICs have been added to a machine.
- Full status with storage.
`)
}

type statusUnitTestSuite struct {
	testhelpers.IsolationSuite
}

func TestStatusUnitTestSuite(t *testing.T) {
	tc.Run(t, &statusUnitTestSuite{})
}

func (s *statusUnitTestSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
-- Note: This comment was taken from the original test suite:
-- // Complete testing of status functionality happens elsewhere in the codebase,
-- // these tests just sanity-check the api itself.
- Status, machines with containers (assert the containers are included in the status' machine list).
- Status, machines with embedded containers (assert the containers are included in the status' machine list).
- Status, applications with exposed endpoints (assert status' endpoints are exposed to the correct spaces and CIDRs).
- Status with subordinates (assert principal applications have their subornidates).
- Status with different versions (assert last version is reported).
- Status with simple workload.
- Status with blank version (assert last version can be blank).
- Status with application with blank version and no units.
- Status with unit with blank version.
- Status when migration is in progress (assert migration in progress status message is reported).
- Status with filtered relations (assert relations are filtered accordig to the pattern passed to status).
- Status with filtered applications, ensure lp#1592872 fix is working.
- Status with cross model relations
- TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly
 tests scenario where applications are returned as part of the status because
 they are related to an application that matches given filter.
 However, the relations for these applications should not be returned.
 In other words, if there are two applications, A and B, such that:

 * an application A matches the supplied filter directly;
 * an application B has units on the same machine as units of an application A and, thus,
 qualifies to be returned by the status result;

 application B's relations should not be returned.
- Status when machine has no display name (assert empty display name in status).
- Status when machine has display name (assert correct display name in status).
- Status when units with opened ports.
`)
}
