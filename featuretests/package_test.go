// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"flag"
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var runFeatureTests = flag.Bool("featuretests", true, "Run long-running feature tests.")

func init() {

	flag.Parse()

	if *runFeatureTests == false {
		return
	}
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
	gc.Suite(&cmdSpaceSuite{})
	gc.Suite(&cmdSubnetSuite{})
}

func Test(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
