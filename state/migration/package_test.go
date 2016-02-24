// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"testing"

	gc "gopkg.in/check.v1"
)

// Useful test constants.

const gig uint64 = 1024 * 1024 * 1024

// None of the tests in this package require mongo.
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
