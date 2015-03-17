// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"github.com/juju/juju/juju/testing"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&workerSuite{})

type workerSuite struct {
	testing.JujuConnSuite
}
