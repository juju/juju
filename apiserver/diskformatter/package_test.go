// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	//TODO(bogdanteleaga): fix this on windows
	if runtime.GOOS == "windows" {
		t.Skip("Diskformatter not supported at the moment on windows")
	}
	gc.TestingT(t)
}
