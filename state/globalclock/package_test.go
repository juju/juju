// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock_test

import (
	"testing"

	coretesting "github.com/juju/juju/v2/testing"
)

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
