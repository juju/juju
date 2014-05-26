// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	"path/filepath"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upgrades"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// assertExpectedSteps is a helper function used to check that the upgrade steps match
// what is expected for a version.
func assertExpectedSteps(c *gc.C, steps []upgrades.Step, expectedSteps []string) {
	var stepNames = make([]string, len(steps))
	for i, step := range steps {
		stepNames[i] = step.Description()
	}
	c.Assert(stepNames, gc.DeepEquals, expectedSteps)
}

type upgradeSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&upgradeSuite{})

type mockUpgradeOperation struct {
	targetVersion version.Number
	steps         []upgrades.Step
}

func (m *mockUpgradeOperation) TargetVersion() version.Number {
	return m.targetVersion
}

func (m *mockUpgradeOperation) Steps() []upgrades.Step {
	return m.steps
}

type mockUpgradeStep struct {
	msg     string
	targets []upgrades.Target
}

func (u *mockUpgradeStep) Description() string {
	return u.msg
}

func (u *mockUpgradeStep) Targets() []upgrades.Target {
	return u.targets
}

func (u *mockUpgradeStep) Run(context upgrades.Context) error {
	if strings.HasSuffix(u.msg, "error") {
		return errors.New("upgrade error occurred")
	}
	ctx := context.(*mockContext)
	ctx.messages = append(ctx.messages, u.msg)
	return nil
}

type mockContext struct {
	messages        []string
	agentConfig     *mockAgentConfig
	realAgentConfig agent.ConfigSetter
	apiState        *api.State
	state           *state.State
}

func (c *mockContext) APIState() *api.State {
	return c.apiState
}

func (c *mockContext) State() *state.State {
	return c.state
}

func (c *mockContext) AgentConfig() agent.ConfigSetter {
	if c.realAgentConfig != nil {
		return c.realAgentConfig
	}
	return c.agentConfig
}

type mockAgentConfig struct {
	agent.ConfigSetter
	dataDir      string
	logDir       string
	tag          string
	jobs         []params.MachineJob
	apiAddresses []string
	values       map[string]string
}

func (mock *mockAgentConfig) Tag() string {
	return mock.tag
}

func (mock *mockAgentConfig) DataDir() string {
	return mock.dataDir
}

func (mock *mockAgentConfig) LogDir() string {
	return mock.logDir
}

func (mock *mockAgentConfig) SystemIdentityPath() string {
	return filepath.Join(mock.dataDir, agent.SystemIdentity)
}

func (mock *mockAgentConfig) Jobs() []params.MachineJob {
	return mock.jobs
}

func (mock *mockAgentConfig) APIAddresses() ([]string, error) {
	return mock.apiAddresses, nil
}

func (mock *mockAgentConfig) Value(name string) string {
	return mock.values[name]
}

func targets(targets ...upgrades.Target) (upgradeTargets []upgrades.Target) {
	for _, t := range targets {
		upgradeTargets = append(upgradeTargets, t)
	}
	return upgradeTargets
}

func upgradeOperations() []upgrades.Operation {
	steps := []upgrades.Operation{
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.12.0"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.12.0", nil},
				&mockUpgradeStep{"step 2 error", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 3", targets(upgrades.HostMachine)},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.16.0"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.16.0", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 2 - 1.16.0", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 3 - 1.16.0", targets(upgrades.StateServer)},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.0"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.17.0", targets(upgrades.HostMachine)},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.1"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.17.1", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 2 - 1.17.1", targets(upgrades.StateServer)},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.18.0"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.18.0", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 2 - 1.18.0", targets(upgrades.StateServer)},
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.20.0"),
			steps: []upgrades.Step{
				&mockUpgradeStep{"step 1 - 1.20.0", targets(upgrades.AllMachines)},
				&mockUpgradeStep{"step 2 - 1.20.0", targets(upgrades.HostMachine)},
				&mockUpgradeStep{"step 3 - 1.20.0", targets(upgrades.StateServer)},
			},
		},
	}
	return steps
}

type upgradeTest struct {
	about         string
	fromVersion   string
	toVersion     string
	target        upgrades.Target
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
		about:         "target version excludes steps for newer version",
		toVersion:     "1.17.1",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.17.0", "step 1 - 1.17.1"},
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
		about:         "allMachines matches everything",
		fromVersion:   "1.18.1",
		toVersion:     "1.20.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0"},
	},
	{
		about:         "allMachines matches everything",
		fromVersion:   "1.18.1",
		toVersion:     "1.20.0",
		target:        upgrades.StateServer,
		expectedSteps: []string{"step 1 - 1.20.0", "step 3 - 1.20.0"},
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
		toVersion := version.MustParse("1.18.0")
		if test.toVersion != "" {
			toVersion = version.MustParse(test.toVersion)
		}
		vers := version.Current
		vers.Number = toVersion
		s.PatchValue(&version.Current, vers)
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
	for i, utv := range (*upgrades.UpgradeOperations)() {
		vers := utv.TargetVersion()
		if i > 0 {
			c.Check(previous.Compare(vers), gc.Equals, -1)
		}
		previous = vers
	}
}

var expectedVersions = []string{"1.18.0"}

func (s *upgradeSuite) TestUpgradeOperationsVersions(c *gc.C) {
	var versions []string
	for _, utv := range (*upgrades.UpgradeOperations)() {
		versions = append(versions, utv.TargetVersion().String())

	}
	c.Assert(versions, gc.DeepEquals, expectedVersions)
}
