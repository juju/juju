// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.
package winrmprovisioner_test

import (
	"runtime"
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

func Test(t *stdtesting.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Manual provider as client is not supported on windows")
	}
	gc.TestingT(t)
}
