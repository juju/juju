// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func init() {
	if os.Getenv("JUJU_FEATURE_TESTS") == "1" || *runFeatureTests {
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
