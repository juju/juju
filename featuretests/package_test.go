// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"os"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
)

func init() {
	if os.Getenv(osenv.JujuFeatureTestsEnvKey) == "1" {
		// Initialize all suites here.
		gc.Suite(&cmdJujuSuite{})
		gc.Suite(&annotationsSuite{})
		gc.Suite(&apiEnvironmentSuite{})
		gc.Suite(&blockSuite{})
		gc.Suite(&apiCharmsSuite{})
		gc.Suite(&cmdEnvironmentSuite{})
		gc.Suite(&cmdStorageSuite{})
		gc.Suite(&cmdSystemSuite{})
		gc.Suite(&dblogSuite{})
		gc.Suite(&cloudImageMetadataSuite{})
	}
}

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
