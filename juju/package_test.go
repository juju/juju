package juju_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
