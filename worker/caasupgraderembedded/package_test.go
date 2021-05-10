// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package caasupgraderembedded

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var ToBinaryVersion = toBinaryVersion
