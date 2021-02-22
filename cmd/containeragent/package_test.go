// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package main

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestAll(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("containeragent only runs on Linux")
	}
	gc.TestingT(t)
}
