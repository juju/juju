// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	coretesting.MgoSSLTestPackage(t)
}
