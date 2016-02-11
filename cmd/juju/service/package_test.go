// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"runtime"
	stdtesting "testing"

	gittesting "github.com/juju/testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

func TestPackage(t *stdtesting.T) {
	if runtime.GOARCH == "386" {
		t.Skipf("skipping package for %v/%v, see http://pad.lv/1425569", runtime.GOOS, runtime.GOARCH)
	}
	if gittesting.RaceEnabled {
		t.Skip("skipping test in -race mode, see LP 1518810")
	}
	testing.MgoTestPackage(t)
}
