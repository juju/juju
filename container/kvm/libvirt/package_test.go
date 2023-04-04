// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	if runtime.GOOS != "linux" || !supportedArch() {
		t.Skip("KVM is currently only supported on linux architectures amd64, arm64, and ppc64el")
	}
	gc.TestingT(t)
}

func supportedArch() bool {
	for _, arch := range []string{"amd64", "arm64", "ppc64el"} {
		if runtime.GOARCH == arch {
			return true
		}
	}
	return false
}
