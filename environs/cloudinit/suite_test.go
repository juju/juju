package cloudinit_test

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type suite struct{}

var _ = Suite(suite{})
