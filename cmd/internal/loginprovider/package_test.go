// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loginprovider

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
