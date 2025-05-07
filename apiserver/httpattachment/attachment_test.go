// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpattachment_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type requestSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&requestSuite{})

// TODO the functions in this package should be tested directly.
// https://bugs.launchpad.net/juju-core/+bug/1503990
