package ec2_test

import (
	"flag"
	"launchpad.net/juju-core/environs/ec2"
	"launchpad.net/juju-core/log"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

var regenerate = flag.Bool("regenerate-images", false, "regenerate all data in images directory")
var amazon = flag.Bool("amazon", false, "Also run some tests on live Amazon servers")

func TestEC2(t *stdtesting.T) {
	log.Debug = true
	if *regenerate {
		ec2.RegenerateImages(t)
	}
	if *amazon {
		registerAmazonTests()
	}
	registerLocalTests()
	coretesting.ZkTestPackage(t)
}
