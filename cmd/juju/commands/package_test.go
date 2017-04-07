// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"runtime"
	stdtesting "testing"

	"github.com/juju/juju/component/all"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func init() {
	if err := all.RegisterForClient(); err != nil {
		panic(err)
	}
}

func TestPackage(t *stdtesting.T) {
	if runtime.GOARCH == "386" {
		t.Skipf("skipping package for %v/%v, see http://pad.lv/1425569", runtime.GOOS, runtime.GOARCH)
	}
	testing.MgoTestPackage(t)
}
