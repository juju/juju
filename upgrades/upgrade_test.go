// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	"strings"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upgrades"
	"launchpad.net/juju-core/version"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type upgradeSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&upgradeSuite{})

type mockUpgradeOperation struct {
	targetVersion version.Number
	steps         []upgrades.UpgradeStep
}

func (m *mockUpgradeOperation) TargetVersion() version.Number {
	return m.targetVersion
}

func (m *mockUpgradeOperation) Steps() []upgrades.UpgradeStep {
	return m.steps
}

type mockUpgradeStep struct {
	msg     string
	targets []upgrades.UpgradeTarget
	context *mockContext
}

func (u *mockUpgradeStep) Description() string {
	return u.msg
}

func (u *mockUpgradeStep) Targets() []upgrades.UpgradeTarget {
	return u.targets
}

func (u *mockUpgradeStep) Run(context upgrades.Context) error {
	if strings.HasSuffix(u.msg, "error") {
		return errors.New("upgrade error occurred")
	}
	u.context.messages = append(u.context.messages, u.msg)
	return nil
}

type mockContext struct {
	messages []string
}

func (c *mockContext) APIState() *api.State {
	return nil
}

func (c *mockContext) AgentConfig() agent.Config {
	return nil
}

func targets(targets ...upgrades.UpgradeTarget) (upgradeTargets []upgrades.UpgradeTarget) {
	for _, t := range targets {
		upgradeTargets = append(upgradeTargets, t)
	}
	return upgradeTargets
}

func upgradeOperations(context upgrades.Context) []upgrades.UpgradeOperation {
	mockContext := context.(*mockContext)
	steps := []upgrades.UpgradeOperation{
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.12.0"),
			steps: []upgrades.UpgradeStep{
				&mockUpgradeStep{"step 1 - 1.12.0", nil, mockContext},
				&mockUpgradeStep{"step 2 error", targets(upgrades.HostMachine), mockContext},
				&mockUpgradeStep{"step 3", targets(upgrades.HostMachine), mockContext},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.16.0"),
			steps: []upgrades.UpgradeStep{
				&mockUpgradeStep{"step 1 - 1.16.0", targets(upgrades.HostMachine), mockContext},
				&mockUpgradeStep{"step 2 - 1.16.0", targets(upgrades.HostMachine), mockContext},
				&mockUpgradeStep{"step 3 - 1.16.0", targets(upgrades.StateServer), mockContext},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.0"),
			steps: []upgrades.UpgradeStep{
				&mockUpgradeStep{"step 1 - 1.17.0", targets(upgrades.HostMachine), mockContext},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.1"),
			steps: []upgrades.UpgradeStep{
				&mockUpgradeStep{"step 1 - 1.17.1", targets(upgrades.HostMachine), mockContext},
				&mockUpgradeStep{"step 2 - 1.17.1", targets(upgrades.StateServer), mockContext},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.18.0"),
			steps: []upgrades.UpgradeStep{
				&mockUpgradeStep{"step 1 - 1.18.0", targets(upgrades.HostMachine), mockContext},
				&mockUpgradeStep{"step 2 - 1.18.0", targets(upgrades.StateServer), mockContext},
			},
		},
	}
	return steps
}

type upgradeTest struct {
	about         string
	fromVersion   string
	target        upgrades.UpgradeTarget
	expectedSteps []string
	err           string
}

var upgradeTests = []upgradeTest{
	{
		about:         "from version excludes steps for same version",
		fromVersion:   "1.18.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{},
	},
	{
		about:         "from version excludes older steps",
		fromVersion:   "1.17.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.17.1", "step 1 - 1.18.0"},
	},
	{
		about:         "incompatible targets excluded",
		fromVersion:   "1.17.1",
		target:        upgrades.StateServer,
		expectedSteps: []string{"step 2 - 1.18.0"},
	},
	{
		about:         "error aborts, subsequent steps not run",
		fromVersion:   "1.10.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.12.0"},
		err:           "step 2 error: upgrade error occurred",
	},
	{
		about:         "default from version is 1.16",
		fromVersion:   "",
		target:        upgrades.StateServer,
		expectedSteps: []string{"step 2 - 1.17.1", "step 2 - 1.18.0"},
	},
}

func (s *upgradeSuite) TestPerformUpgrade(c *gc.C) {
	s.PatchValue(upgrades.UpgradeOperations, upgradeOperations)
	for i, test := range upgradeTests {
		c.Logf("%d: %s", i, test.about)
		var messages []string
		ctx := &mockContext{
			messages: messages,
		}
		fromVersion := version.Zero
		if test.fromVersion != "" {
			fromVersion = version.MustParse(test.fromVersion)
		}
		err := upgrades.PerformUpgrade(fromVersion, test.target, ctx)
		if test.err == "" {
			c.Check(err, gc.IsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(ctx.messages, jc.DeepEquals, test.expectedSteps)
	}
}

func (s *upgradeSuite) TestUpgradeOperationsOrdered(c *gc.C) {
	var previous version.Number
	for i, utv := range (*upgrades.UpgradeOperations)(nil) {
		vers := utv.TargetVersion()
		if i > 0 {
			c.Check(previous.Less(vers), jc.IsTrue)
		}
		previous = vers
	}
}
