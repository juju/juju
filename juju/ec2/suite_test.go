package ec2

import (
	. "launchpad.net/gocheck"
	"testing"
)

type suite struct{}

var _ = Suite(suite{})

func TestEC2(t *testing.T) {
	TestingT(t)
}
