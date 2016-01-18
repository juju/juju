// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	//TODO(bogdanteleaga): Fix this once manual provider is supported on
	//windows
	if runtime.GOOS == "windows" {
		t.Skip("Manual provider is not yet supported on windows")
	}
	gc.TestingT(t)
}
