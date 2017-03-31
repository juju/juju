// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"flag"
	"runtime"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/component/all"
	coretesting "github.com/juju/juju/testing"
)

var runFeatureTests = flag.Bool("featuretests", true, "Run long-running feature tests.")

func init() {
	// Required for anything requiring components (e.g. resources).
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}

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
	gc.Suite(&cmdCredentialSuite{})
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
	gc.Suite(&remoteRelationsSuite{})
	gc.Suite(&crossmodelSuite{})
	gc.Suite(&ApplicationConfigSuite{})
	gc.Suite(&CharmUpgradeSuite{})

	// TODO (anastasiamac 2016-07-19) Bug#1603585
	// These tests cannot run on windows - they require a bootstrapped controller.
	if runtime.GOOS == "linux" {
		gc.Suite(&cloudImageMetadataSuite{})
		gc.Suite(&cmdSpaceSuite{})
	}
}

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

func runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	// Writers need to be reset, because
	// they are set globally in the juju/cmd package and will
	// return an error if we attempt to run two commands in the
	// same test.
	loggo.ResetWriters()
	ctx := coretesting.Context(c)
	command := jujucmd.NewJujuCommand(ctx)
	return coretesting.RunCommand(c, command, args...)
}

func runCommandExpectSuccess(c *gc.C, command string, args ...string) {
	_, err := runCommand(c, append([]string{command}, args...)...)
	c.Assert(err, jc.ErrorIsNil)
}

func runCommandExpectFailure(c *gc.C, command, expectedError string, args ...string) {
	context, err := runCommand(c, append([]string{command}, args...)...)
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(coretesting.Stderr(context), jc.Contains, expectedError)
}
