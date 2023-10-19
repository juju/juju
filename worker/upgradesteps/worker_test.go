// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
)

// TODO(mjs) - these tests are too tightly coupled to the
// implementation. They needn't be internal tests.

type UpgradeSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&UpgradeSuite{})
