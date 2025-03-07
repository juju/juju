// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
