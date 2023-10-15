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
	gc.Suite(&dblogSuite{})
	gc.Suite(&dumpLogsCommandSuite{})
	gc.Suite(&undertakerSuite{})
	gc.Suite(&debugLogDbSuite1{})
	gc.Suite(&debugLogDbSuite2{})
	gc.Suite(&meterStatusIntegrationSuite{})
	gc.Suite(&toolsDownloadSuite{})
	gc.Suite(&toolsWithMacaroonsSuite{})
	gc.Suite(&CredentialManagerSuite{})
	gc.Suite(&ControllerSuite{})
}

func Test(t *testing.T) {
	coretesting.MgoSSLTestPackage(t)
}
