// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"flag"
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package ec2 -destination context_mock_test.go github.com/juju/juju/environs/context ProviderCallContext

var amazon = flag.Bool("amazon", false, "Also run some tests on live Amazon servers")

func TestPackage(t *stdtesting.T) {
	if *amazon {
		registerAmazonTests()
	}
	registerLocalTests()
	gc.TestingT(t)
}
