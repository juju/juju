// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
        stdtesting "testing"
        "time"

        gc "launchpad.net/gocheck"

        "launchpad.net/juju-core/juju/testing"
        "launchpad.net/juju-core/testing/testbase"
        coretesting "launchpad.net/juju-core/testing"
        "launchpad.net/juju-core/worker/resumer"
)

type aggregateSuite struct {
    testbase.LoggingSuite
}

var _ = gc.Suite(&aggregateSuite{})


