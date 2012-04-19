package ec2_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"testing"
)

var regenerate = flag.Bool("regenerate-images", false, "regenerate all data in images directory")
var amazon = flag.Bool("amazon", false, "Also run some tests on live Amazon servers")

func TestEC2(t *testing.T) {
	if *regenerate {
		regenerateImages(t)
	}
	if *amazon {
		registerAmazonTests()
	}
	registerLocalTests()
	TestingT(t)
}
