// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: this is not supported on windows atm")
	}
	gc.TestingT(t)
}
