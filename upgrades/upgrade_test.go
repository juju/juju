// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// assertSteps is a helper that ensures that the given upgrade steps
// match what is expected for that version and that the steps have
// been added to the global upgrade operations list.
func assertSteps(c *gc.C, ver version.Number, expectedStateSteps, expectedSteps []string) {
	for _, op := range (*upgrades.UpgradeOperations)() {
		if op.TargetVersion() == ver {
			assertExpectedSteps(c, upgrades.GetStateSteps(op), expectedStateSteps)
			assertExpectedSteps(c, upgrades.GetSteps(op), expectedSteps)
			return
		}
	}
	c.Fatal("upgrade operations for this version are not hooked up")
}

// assertExpectedSteps is a helper function used to check that the upgrade steps match
// what is expected for a version.
func assertExpectedSteps(c *gc.C, steps []upgrades.GenericStep, expectedSteps []string) {
	c.Assert(steps, gc.HasLen, len(expectedSteps))

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
	stateSteps    []upgrades.StateStep
	steps         []upgrades.Step
}

func (m *mockUpgradeOperation) TargetVersion() version.Number {
	return m.targetVersion
}

func (m *mockUpgradeOperation) StateSteps() []upgrades.StateStep {
	return m.stateSteps
}

func (m *mockUpgradeOperation) Steps() []upgrades.Step {
	return m.steps
}

type mockBaseUpgradeStep struct {
	msg     string
	targets []upgrades.Target
}

func (u *mockBaseUpgradeStep) Description() string {
	return u.msg
}

func (u *mockBaseUpgradeStep) Targets() []upgrades.Target {
	return u.targets
}

func (u *mockBaseUpgradeStep) run(context *mockContext) error {
	if strings.HasSuffix(u.msg, "error") {
		return errors.New("upgrade error occurred")
	}
	context.messages = append(context.messages, u.msg)
	return nil
}

func newStateUpgradeStep(msg string, targets ...upgrades.Target) *mockStateUpgradeStep {
	return &mockStateUpgradeStep{mockBaseUpgradeStep{msg, targets}}
}

type mockStateUpgradeStep struct {
	mockBaseUpgradeStep
}

func (u *mockStateUpgradeStep) Run(context upgrades.StateContext) error {
	return u.run(context.(*mockContext))
}

func newUpgradeStep(msg string, targets ...upgrades.Target) *mockUpgradeStep {
	return &mockUpgradeStep{mockBaseUpgradeStep{msg, targets}}
}

type mockUpgradeStep struct {
	mockBaseUpgradeStep
}

func (u *mockUpgradeStep) Run(context upgrades.APIContext) error {
	return u.run(context.(*mockContext))
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

func (c *mockContext) StateContext() upgrades.StateContext {
	return c
}

type mockAgentConfig struct {
	agent.ConfigSetter
	dataDir      string
	logDir       string
	tag          names.Tag
	jobs         []params.MachineJob
	apiAddresses []string
	values       map[string]string
	mongoInfo    *mongo.MongoInfo
}

func (mock *mockAgentConfig) Tag() names.Tag {
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

func (mock *mockAgentConfig) MongoInfo() (*mongo.MongoInfo, bool) {
	return mock.mongoInfo, true
}

func upgradeOperations() []upgrades.Operation {
	steps := []upgrades.Operation{
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.11.0"),
			stateSteps: []upgrades.StateStep{
				newStateUpgradeStep("state step 1 - 1.11.0"),
				newStateUpgradeStep("state step 2 error", upgrades.StateServer),
				newStateUpgradeStep("state step 3 - 1.11.0", upgrades.StateServer),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.12.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.12.0"),
				newUpgradeStep("step 2 error", upgrades.HostMachine),
				newUpgradeStep("step 3", upgrades.HostMachine),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.16.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.16.0", upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.16.0", upgrades.HostMachine),
				newUpgradeStep("step 3 - 1.16.0", upgrades.StateServer),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.17.0", upgrades.HostMachine),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.17.1"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.17.1", upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.17.1", upgrades.StateServer),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.18.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.18.0", upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.18.0", upgrades.StateServer),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.20.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.20.0", upgrades.AllMachines),
				newUpgradeStep("step 2 - 1.20.0", upgrades.HostMachine),
				newUpgradeStep("step 3 - 1.20.0", upgrades.StateServer),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.21.0"),
			stateSteps: []upgrades.StateStep{
				newStateUpgradeStep("state step 1 - 1.21.0", upgrades.DatabaseMaster),
				newStateUpgradeStep("state step 2 - 1.21.0", upgrades.StateServer),
			},
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.21.0", upgrades.AllMachines),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.22.0"),
			stateSteps: []upgrades.StateStep{
				newStateUpgradeStep("state step 1 - 1.22.0", upgrades.DatabaseMaster),
				newStateUpgradeStep("state step 2 - 1.22.0", upgrades.StateServer),
			},
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.22.0", upgrades.AllMachines),
				newUpgradeStep("step 2 - 1.22.0", upgrades.AllMachines),
			},
		},
	}
	return steps
}

type areUpgradesDefinedTest struct {
	about       string
	fromVersion string
	toVersion   string
	expected    bool
	err         string
}

var areUpgradesDefinedTests = []areUpgradesDefinedTest{
	{
		about:       "no ops if same version",
		fromVersion: "1.18.0",
		expected:    false,
	},
	{
		about:       "true when ops defined between versions",
		fromVersion: "1.17.1",
		expected:    true,
	},
	{
		about:       "false when no ops defined between versions",
		fromVersion: "1.13.0",
		toVersion:   "1.14.1",
		expected:    false,
	},
	{
		about:       "from version is defaulted when not supplied",
		fromVersion: "",
		expected:    true,
	},
}

func (s *upgradeSuite) TestAreUpgradesDefined(c *gc.C) {
	s.PatchValue(upgrades.UpgradeOperations, upgradeOperations)
	for i, test := range areUpgradesDefinedTests {
		c.Logf("%d: %s", i, test.about)
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
		result := upgrades.AreUpgradesDefined(fromVersion)
		c.Check(result, gc.Equals, test.expected)
	}
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
		about:         "the database master target is also a state server",
		fromVersion:   "1.18.1",
		toVersion:     "1.20.0",
		target:        upgrades.DatabaseMaster,
		expectedSteps: []string{"step 1 - 1.20.0", "step 3 - 1.20.0"},
	},
	{
		about:         "state step error aborts, subsequent state steps not run",
		fromVersion:   "1.10.0",
		target:        upgrades.StateServer,
		expectedSteps: []string{"state step 1 - 1.11.0"},
		err:           "state step 2 error: upgrade error occurred",
	},
	{
		about:         "error aborts, subsequent steps not run",
		fromVersion:   "1.11.0",
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
	{
		about:         "state servers don't get database master",
		fromVersion:   "1.20.0",
		toVersion:     "1.21.0",
		target:        upgrades.StateServer,
		expectedSteps: []string{"state step 2 - 1.21.0", "step 1 - 1.21.0"},
	},
	{
		about:       "database masters are state servers",
		fromVersion: "1.20.0",
		toVersion:   "1.21.0",
		target:      upgrades.DatabaseMaster,
		expectedSteps: []string{
			"state step 1 - 1.21.0", "state step 2 - 1.21.0",
			"step 1 - 1.21.0",
		},
	},
	{
		about:       "all state steps are run first",
		fromVersion: "1.20.0",
		toVersion:   "1.22.0",
		target:      upgrades.DatabaseMaster,
		expectedSteps: []string{
			"state step 1 - 1.21.0", "state step 2 - 1.21.0",
			"state step 1 - 1.22.0", "state step 2 - 1.22.0",
			"step 1 - 1.21.0",
			"step 1 - 1.22.0", "step 2 - 1.22.0",
		},
	},
	{
		about:         "upgrade to alpha release runs steps for final release",
		fromVersion:   "1.20.0",
		toVersion:     "1.21-alpha1",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.21.0"},
	},
	{
		about:         "upgrade to beta release runs steps for final release",
		fromVersion:   "1.20.0",
		toVersion:     "1.21-beta2",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.21.0"},
	},
	{
		about:         "starting release steps included when upgrading from an alpha release",
		fromVersion:   "1.20-alpha3",
		toVersion:     "1.21.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0", "step 1 - 1.21.0"},
	},
	{
		about:         "starting release steps included when upgrading from an beta release",
		fromVersion:   "1.20-beta1",
		toVersion:     "1.21.0",
		target:        upgrades.HostMachine,
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0", "step 1 - 1.21.0"},
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

func (s *upgradeSuite) TestStateStepsNotAttemptedWhenNoStateTarget(c *gc.C) {
	count := 0
	upgradeOperations := func() []upgrades.Operation {
		count++
		return nil
	}
	s.PatchValue(upgrades.UpgradeOperations, upgradeOperations)

	fromVers := version.MustParse("1.18.0")
	ctx := new(mockContext)
	check := func(target upgrades.Target, expectedCallCount int) {
		count = 0
		err := upgrades.PerformUpgrade(fromVers, target, ctx)
		c.Assert(err, gc.IsNil)
		c.Assert(count, gc.Equals, expectedCallCount)
	}

	// Expect 2 iterations through the list of operations when the
	// target could involve updates to State directly; 1 otherwise.
	check(upgrades.StateServer, 2)
	check(upgrades.DatabaseMaster, 2)
	check(upgrades.AllMachines, 1)
	check(upgrades.HostMachine, 1)
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

var expectedVersions = []string{"1.18.0", "1.21.0"}

func (s *upgradeSuite) TestUpgradeOperationsVersions(c *gc.C) {
	var versions []string
	for _, utv := range (*upgrades.UpgradeOperations)() {
		vers := utv.TargetVersion()
		// Upgrade steps should only be targeted at final versions (not alpha/beta).
		c.Check(vers.Tag, gc.Equals, "")
		versions = append(versions, vers.String())

	}
	c.Assert(versions, gc.DeepEquals, expectedVersions)
}
