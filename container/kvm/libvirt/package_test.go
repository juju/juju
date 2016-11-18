// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("KVM is currently not supported on windows")
	}
	gc.TestingT(t)
}
