// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(dimitern) Disabled on gccgo (PPC64 in particular) due
// to build failures. See bug http://pad.lv/1425788.
//
// NOTE: This file is built only with the default gc compiler. We
// can't use runtime.Compiler to exclude only the problematic
// gc.Suite(&cmdJujuSuite{}) statement below because runtime.Compiler
// is a const, whereas the +build directives are honored by -compiler
// gccgo as well. See also the comment in package_gccgo_test.go.

// +build !gccgo

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
	gc.Suite(&cmdJujuSuite{})
	gc.Suite(&leadershipSuite{})
	gc.Suite(&uniterLeadershipSuite{})
}

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
