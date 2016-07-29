// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"flag"
	"runtime"
	stdtesting "testing"

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
	gc.Suite(&annotationsSuite{})
	gc.Suite(&CloudAPISuite{})
	gc.Suite(&apiEnvironmentSuite{})
	gc.Suite(&BakeryStorageSuite{})
	gc.Suite(&blockSuite{})
	gc.Suite(&cmdControllerSuite{})
	gc.Suite(&cmdJujuSuite{})
	gc.Suite(&cmdLoginSuite{})
	gc.Suite(&cmdModelSuite{})
	gc.Suite(&cmdRegistrationSuite{})
	gc.Suite(&cmdStorageSuite{})
	gc.Suite(&cmdSubnetSuite{})
	gc.Suite(&dblogSuite{})
	gc.Suite(&dumpLogsCommandSuite{})
	gc.Suite(&undertakerSuite{})
	gc.Suite(&upgradeSuite{})

	// TODO (anastasiamac 2016-07-19) Bug#1603585
	// These tests cannot run on windows - they require a bootstrapped controller.
	if runtime.GOOS == "linux" {
		gc.Suite(&cloudImageMetadataSuite{})
		gc.Suite(&cmdSpaceSuite{})
	}
}

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
