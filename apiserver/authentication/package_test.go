// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"testing"

	coretesting "github.com/juju/juju/v2/testing"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
