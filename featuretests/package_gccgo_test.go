// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(dimitern) Remove this file once bug http://pad.lv/1425788 is
// resolved. For now in order to exclude the problematic
// cmdjuju_test.go from compiling on gccgo (amd64 and ppc64 are both
// affected), while also running the other tests which pass on gccgo
// we need 2 package_test files: this one needs to be called
// package_gccgo_test.go, not package_test_gccgo.go in order to be
// considered at all by 'go test' which ignores non *_test.go files.

// +build gccgo

package featuretests

import (
	"flag"
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var runFeatureTests = flag.Bool("featuretests", true, "Run long-running feature tests.")

func init() {

	flag.Parse()

	if *runFeatureTests == false {
		return
	}
	// Initialize all suites here.
	gc.Suite(&leadershipSuite{})
	gc.Suite(&uniterLeadershipSuite{})
}

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
