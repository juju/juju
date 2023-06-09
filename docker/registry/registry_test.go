// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registrySuite{})
