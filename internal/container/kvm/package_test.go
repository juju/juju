// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
)

func Test(t *testing.T) {
	if runtime.GOOS != "linux" || !supportedArch() {
		t.Skip("KVM is currently only supported on linux architectures amd64, arm64, and ppc64el")
	}
	gc.TestingT(t)
}

func supportedArch() bool {
	for _, a := range []string{arch.AMD64, arch.ARM64, arch.PPC64EL} {
		if runtime.GOARCH == a {
			return true
		}
	}
	return false
}
