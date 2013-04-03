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
