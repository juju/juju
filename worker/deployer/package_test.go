// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"runtime"
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: Currently does not work under windows")
	}
	coretesting.MgoTestPackage(t)
}
