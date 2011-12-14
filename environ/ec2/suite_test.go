package ec2

import (
	"flag"
	. "launchpad.net/gocheck"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

var regenerate = flag.Bool("regenerate-images", false, "regenerate all data in images directory")

func TestEC2(t *testing.T) {
	if *regenerate {
		regenerateImages(t)
	}
	TestingT(t)
}
