package ec2_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

var regenerate = flag.Bool("regenerate-images", false, "regenerate all data in images directory")
var integration = flag.Bool("integration", false, "Enable integration tests")

func TestEC2(t *testing.T) {
	if *regenerate {
		regenerateImages(t)
	}
	if *integration {
		registerIntegrationTests()
	}
	registerLocalTests()
	TestingT(t)
}
