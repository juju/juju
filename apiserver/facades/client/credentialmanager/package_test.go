// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
