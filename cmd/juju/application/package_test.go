// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"runtime"
	stdtesting "testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

func TestPackage(t *stdtesting.T) {
	if runtime.GOARCH == "386" {
		t.Skipf("skipping package for %v/%v, see http://pad.lv/1425569", runtime.GOOS, runtime.GOARCH)
	}
	testing.MgoTestPackage(t)
}
