// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
