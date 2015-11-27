// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"flag"
	stdtesting "testing"

	"github.com/juju/testing"

	gc "gopkg.in/check.v1"
)

var amazon = flag.Bool("amazon", false, "Also run some tests on live Amazon servers")

func TestPackage(t *stdtesting.T) {
	if testing.RaceEnabled {
		t.Skip("skipping package under -race, see LP 1519141")
	}
	if *amazon {
		registerAmazonTests()
	}
	registerLocalTests()
	gc.TestingT(t)
}
