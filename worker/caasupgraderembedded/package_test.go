// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package caasupgraderembedded

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var ToBinaryVersion = toBinaryVersion
