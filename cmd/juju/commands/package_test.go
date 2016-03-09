// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"flag"
	"runtime"
	stdtesting "testing"

	gittesting "github.com/juju/testing"

	cmdtesting "github.com/juju/juju/cmd/testing"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	if runtime.GOARCH == "386" {
		t.Skipf("skipping package for %v/%v, see http://pad.lv/1425569", runtime.GOOS, runtime.GOARCH)
	}
	if gittesting.RaceEnabled {
		t.Skip("skipping test in -race mode, see LP 1518810")
	}
	testing.MgoTestPackage(t)
}

// Reentrancy point for testing (something as close as possible to) the juju
// tool itself.
func TestRunMain(t *stdtesting.T) {
	if *cmdtesting.FlagRunMain {
		Main(flag.Args())
	}
}
