// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/apiserver/client"
)

type runSuite struct {
	baseSuite
}

var _ = gc.Suite(&runSuite{})
