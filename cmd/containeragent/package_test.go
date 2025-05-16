// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"runtime"
	stdtesting "testing"

	"github.com/juju/tc"
)

func TestAll(t *stdtesting.T) {
	if runtime.GOOS != "linux" {
		t.Skip("containeragent only runs on Linux")
	}
	tc.TestingT(t)
}
