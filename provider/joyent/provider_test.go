// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	"flag"
	"testing"

	gc "launchpad.net/gocheck"
)

var live = flag.Bool("live", false, "Also run tests on live Joyent Public Cloud")

func TestJoyent(t *testing.T) {
	if *live {
		registerLiveTests()
	}
	registerLocalTests()
	gc.TestingT(t)
}
