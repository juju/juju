// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"flag"
	"runtime"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd/juju/commands"
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
	gc.Suite(&CmdRelationSuite{})

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

func runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	// Writers need to be reset, because
	// they are set globally in the juju/cmd package and will
	// return an error if we attempt to run two commands in the
	// same test.
	loggo.ResetWriters()
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	command := jujucmd.NewJujuCommand(ctx)
	return coretesting.RunCommand(c, command, args...)
}
