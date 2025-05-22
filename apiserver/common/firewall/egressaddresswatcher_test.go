// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestAddressWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &addressWatcherSuite{})
}

type addressWatcherSuite struct {
	coretesting.BaseSuite
}

func (s *addressWatcherSuite) TestWatchStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
	- unit in the scope before the watcher start
	- unit enters scope
	- unit leaves scope
    - two units enters scope at the same time
    - one unit then another units enters scope
    - unit enters scope with no public address
    - not assigned unit enters scope
    - unit leaves scope without having seen by enter scope
    - two units with same address one leaves, then the other leaves
    - unit update its address
    - a unit address change event has been raised without any changes
    - the machine address changes after the unit has been departed
    - test the egress address defaulted to model egress address if no relation address
    - test the egress address used relation address if not empty
`)
}
