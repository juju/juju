// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

// None of the tests in this package require mongo.
// Full command integration tests are found in featuretests package.

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
