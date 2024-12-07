// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type filteringStatusSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&filteringStatusSuite{})

func (s *filteringStatusSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- A correctly filtered status is returned when filtering by application name.
- A correctly filtered status is returned when filtering by application name independently of the alphabetical unit ordering. Fix for lp#1592872.
- TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly
tests scenario where applications are returned as part of the status because
they are related to an application that matches given filter.
However, the relations for these applications should not be returned.
In other words, if there are two applications, A and B, such that:
* an application A matches the supplied filter directly;
* an application B has units on the same machine as units of an application A and, thus,
qualifies to be returned by the status result;

application B's relations should not be returned.
- A correctly filtered status is returned when filtering by port range.
`)
}
