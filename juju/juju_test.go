package juju_test

import (
	C "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) {
	C.TestingT(t)
}

type suite struct{}

var _ = C.Suite(suite{})
