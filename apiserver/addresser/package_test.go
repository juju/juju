// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"

	// gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	// gc.TestingT(t)
	coretesting.MgoTestPackage(t)

}
