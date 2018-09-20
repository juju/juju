// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"flag"
	"runtime"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
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
	gc.Suite(&CmdCredentialSuite{})
	gc.Suite(&cmdJujuSuite{})
	gc.Suite(&cmdJujuSuiteNoCAAS{})
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
	gc.Suite(&ResourcesCmdSuite{})
	gc.Suite(&cmdUpdateSeriesSuite{})
	gc.Suite(&FirewallRulesSuite{})
	gc.Suite(&introspectionSuite{})
	gc.Suite(&debugLogDbSuite1{})
	gc.Suite(&debugLogDbSuite2{})
	gc.Suite(&InitiateSuite{})
	gc.Suite(&UserSuite{})
	gc.Suite(&cmdMetricsCommandSuite{})
	gc.Suite(&meterStatusIntegrationSuite{})
	gc.Suite(&CAASOperatorSuite{})
	gc.Suite(&StatusSuite{})
	gc.Suite(&cmdSetSeriesSuite{})
	gc.Suite(&CmdExportBundleSuite{})
	gc.Suite(&cmdDeploySuite{})

	// TODO (anastasiamac 2016-07-19) Bug#1603585
	// These tests cannot run on windows - they require a bootstrapped controller.
	if runtime.GOOS == "linux" {
		gc.Suite(&cloudImageMetadataSuite{})
		gc.Suite(&cmdSpaceSuite{})
		gc.Suite(&cmdUpgradeSuite{})
	}
}

func TestPackage(t *testing.T) {
	coretesting.MgoSSLTestPackage(t)
}

func runCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	// Writers need to be reset, because
	// they are set globally in the juju/cmd package and will
	// return an error if we attempt to run two commands in the
	// same test.
	loggo.ResetWriters()
	ctx := cmdtesting.Context(c)
	command := jujucmd.NewJujuCommand(ctx)
	return cmdtesting.RunCommand(c, command, args...)
}

func runCommandExpectSuccess(c *gc.C, command string, args ...string) {
	_, err := runCommand(c, append([]string{command}, args...)...)
	c.Assert(err, jc.ErrorIsNil)
}

func runCommandExpectFailure(c *gc.C, command, expectedError string, args ...string) {
	context, err := runCommand(c, append([]string{command}, args...)...)
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stderr(context), jc.Contains, expectedError)
}
