// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"runtime"
	"testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd/juju/commands"
	coretesting "github.com/juju/juju/testing"
)

func init() {
	// Initialize all suites here.
	gc.Suite(&annotationsSuite{})
	gc.Suite(&cmdApplicationSuite{})
	gc.Suite(&CloudAPISuite{})
	gc.Suite(&apiModelSuite{})
	gc.Suite(&apiLoggerSuite{})
	gc.Suite(&BakeryStorageSuite{})
	gc.Suite(&blockSuite{})
	gc.Suite(&cmdControllerSuite{})
	gc.Suite(&CmdCredentialSuite{})
	gc.Suite(&CmdCloudSuite{})
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
	gc.Suite(&debugLogDbSuite1{})
	gc.Suite(&debugLogDbSuite2{})
	gc.Suite(&remoteRelationsSuite{})
	gc.Suite(&crossmodelSuite{})
	gc.Suite(&ApplicationConfigSuite{})
	gc.Suite(&CharmUpgradeSuite{})
	gc.Suite(&ResourcesCmdSuite{})
	gc.Suite(&FirewallRulesSuite{})
	gc.Suite(&introspectionSuite{})
	gc.Suite(&InitiateSuite{})
	gc.Suite(&UserSuite{})
	gc.Suite(&cmdMetricsCommandSuite{})
	gc.Suite(&meterStatusIntegrationSuite{})
	gc.Suite(&CAASOperatorSuite{})
	gc.Suite(&StatusSuite{})
	gc.Suite(&cmdSetSeriesSuite{})
	gc.Suite(&cmdExportBundleSuite{})
	gc.Suite(&cmdDeploySuite{})
	gc.Suite(&CredentialManagerSuite{})
	gc.Suite(&cmdCurrentControllerSuite{})

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
	// Writers need to be reset, because they are set globally in the
	// juju/cmd package and will return an error if we attempt to run
	// two commands in the same test.
	loggo.ResetWriters()
	ctx := cmdtesting.Context(c)
	command := jujucmd.NewJujuCommand(ctx, "")
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
