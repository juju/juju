// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func init() {
	// Initialize all suites here.
	gc.Suite(&apiLoggerSuite{})
	gc.Suite(&meterStatusIntegrationSuite{})
	gc.Suite(&toolsWithMacaroonsSuite{})
	gc.Suite(&CredentialManagerSuite{})
	gc.Suite(&ControllerSuite{})
}

func TestPackage(t *testing.T) {
	coretesting.MgoSSLTestPackage(t)
}
