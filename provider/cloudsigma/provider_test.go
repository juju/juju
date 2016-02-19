// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"testing"

	gc "gopkg.in/check.v1"

	tt "github.com/juju/juju/testing"
)

func TestCloudSigma(t *testing.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	tt.BaseSuite
}

var _ = gc.Suite(&providerSuite{})
