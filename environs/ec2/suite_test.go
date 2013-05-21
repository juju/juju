// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"testing"
)

var amazon = flag.Bool("amazon", false, "Also run some tests on live Amazon servers")

func TestEC2(t *testing.T) {
	if *amazon {
		registerAmazonTests()
	}
	registerLocalTests()
	TestingT(t)
}
