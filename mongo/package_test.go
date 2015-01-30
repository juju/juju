// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	//TODO(bogdanteleaga): Fix these on windows
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: Skipping for now on windows")
	}
	gc.TestingT(t)
}
