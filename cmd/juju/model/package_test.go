// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	"github.com/juju/tc"
)

// None of the tests in this package require mongo.
// Full command integration tests are found in cmd/juju/model_test.go

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
