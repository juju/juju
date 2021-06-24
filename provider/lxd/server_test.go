// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

var (
	_ = gc.Suite(&serverSuite{})
)

// serverSuite tests server module functionality from inside the
// lxd package. See server_integration_test.go for tests that use
// only the exported surface of the package.
type serverSuite struct {
	testing.IsolationSuite
}
