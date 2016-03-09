// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type undertakerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&undertakerSuite{})
