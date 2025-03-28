// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

// Untl we add upgrade steps for 3.0, keep static analysis happy.
var _ = findStateStep

func findStateStep(c *gc.C, ver version.Number, description string) upgrades.Step {
	for _, op := range (*upgrades.StateUpgradeOperations)() {
		if op.TargetVersion() == ver {
			for _, step := range op.Steps() {
				if step.Description() == description {
					return step
				}
			}
		}
	}
	c.Fatalf("could not find state step %q for %s", description, ver)
	return nil
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

func (u *mockUpgradeStep) Run(ctx upgrades.Context) error {
	if strings.HasSuffix(u.msg, "error") {
		return errors.New("upgrade error occurred")
	}
	context := ctx.(*mockContext)
	context.messages = append(context.messages, u.msg)
	return nil
}

func newUpgradeStep(msg string, targets ...upgrades.Target) *mockUpgradeStep {
	if len(targets) < 1 {
		panic(fmt.Sprintf("step %q must have at least one target", msg))
	}
	return &mockUpgradeStep{
		msg:     msg,
		targets: targets,
	}
}

type mockContext struct {
	messages        []string
	agentConfig     *mockAgentConfig
	realAgentConfig agent.ConfigSetter
	apiState        api.Connection
	state           upgrades.StateBackend
}

func (c *mockContext) APIState() base.APICaller {
	return c.apiState
}

func (c *mockContext) State() upgrades.StateBackend {
	return c.state
}

func (c *mockContext) AgentConfig() agent.ConfigSetter {
	if c.realAgentConfig != nil {
		return c.realAgentConfig
	}
	return c.agentConfig
}

func (c *mockContext) StateContext() upgrades.Context {
	return c
}

func (c *mockContext) APIContext() upgrades.Context {
	return c
}

type mockAgentConfig struct {
	agent.ConfigSetter
	dataDir      string
	logDir       string
	tag          names.Tag
	jobs         []model.MachineJob
	apiAddresses []string
	values       map[string]string
	mongoInfo    *mongo.MongoInfo
	servingInfo  controller.StateServingInfo
	modelTag     names.ModelTag
}

func (mock *mockAgentConfig) Tag() names.Tag {
	return mock.tag
}

func (mock *mockAgentConfig) DataDir() string {
	return mock.dataDir
}

func (mock *mockAgentConfig) TransientDataDir() string {
	return filepath.Join(mock.dataDir, "transient")
}

func (mock *mockAgentConfig) LogDir() string {
	return mock.logDir
}

func (mock *mockAgentConfig) SystemIdentityPath() string {
	return filepath.Join(mock.dataDir, agent.SystemIdentity)
}

func (mock *mockAgentConfig) Jobs() []model.MachineJob {
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

func (mock *mockAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return mock.servingInfo, true
}

func (mock *mockAgentConfig) SetStateServingInfo(info controller.StateServingInfo) {
	mock.servingInfo = info
}

func (mock *mockAgentConfig) Model() names.ModelTag {
	return mock.modelTag
}

type mockStateBackend struct {
	upgrades.StateBackend
	testing.Stub
}

func (mock *mockStateBackend) ControllerUUID() (string, error) {
	mock.MethodCall(mock, "ControllerUUID")
	return "a-b-c-d", mock.Stub.NextErr()
}

func stateUpgradeOperations() []upgrades.Operation {
	steps := []upgrades.Operation{
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.11.0"),
			steps: []upgrades.Step{
				newUpgradeStep("state step 1 - 1.11.0", upgrades.Controller),
				newUpgradeStep("state step 2 error", upgrades.Controller),
				newUpgradeStep("state step 3 - 1.11.0", upgrades.Controller),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.21.0"),
			steps: []upgrades.Step{
				newUpgradeStep("state step 1 - 1.21.0", upgrades.DatabaseMaster),
				newUpgradeStep("state step 2 - 1.21.0", upgrades.Controller),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.22.0"),
			steps: []upgrades.Step{
				newUpgradeStep("state step 1 - 1.22.0", upgrades.DatabaseMaster),
				newUpgradeStep("state step 2 - 1.22.0", upgrades.Controller),
			},
		},
	}
	return steps
}

func upgradeOperations() []upgrades.Operation {
	steps := []upgrades.Operation{
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.12.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.12.0", upgrades.AllMachines),
				newUpgradeStep("step 2 error", upgrades.HostMachine),
				newUpgradeStep("step 3", upgrades.HostMachine),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.16.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.16.0", upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.16.0", upgrades.HostMachine),
				newUpgradeStep("step 3 - 1.16.0", upgrades.Controller),
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
				newUpgradeStep("step 2 - 1.17.1", upgrades.Controller),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.18.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.18.0", upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.18.0", upgrades.Controller),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.20.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.20.0", upgrades.AllMachines),
				newUpgradeStep("step 2 - 1.20.0", upgrades.HostMachine),
				newUpgradeStep("step 3 - 1.20.0", upgrades.Controller),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.21.0"),
			steps: []upgrades.Step{
				newUpgradeStep("step 1 - 1.21.0", upgrades.AllMachines),
			},
		},
		&mockUpgradeOperation{
			targetVersion: version.MustParse("1.22.0"),
			steps: []upgrades.Step{
				// Separate targets used intentionally
				newUpgradeStep("step 1 - 1.22.0", upgrades.Controller, upgrades.HostMachine),
				newUpgradeStep("step 2 - 1.22.0", upgrades.AllMachines),
			},
		},
	}
	return steps
}

type upgradeTest struct {
	about         string
	fromVersion   string
	toVersion     string
	targets       []upgrades.Target
	expectedSteps []string
	err           string
}

func targets(t ...upgrades.Target) []upgrades.Target {
	return t
}

var upgradeTests = []upgradeTest{
	{
		about:         "from version excludes steps for same version",
		fromVersion:   "1.18.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{},
	},
	{
		about:         "target version excludes steps for newer version",
		toVersion:     "1.17.1",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.17.0", "step 1 - 1.17.1"},
	},
	{
		about:         "from version excludes older steps",
		fromVersion:   "1.17.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.17.1", "step 1 - 1.18.0"},
	},
	{
		about:         "incompatible targets excluded",
		fromVersion:   "1.17.1",
		targets:       targets(upgrades.Controller),
		expectedSteps: []string{"step 2 - 1.18.0"},
	},
	{
		about:         "allMachines matches everything",
		fromVersion:   "1.18.1",
		toVersion:     "1.20.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0"},
	},
	{
		about:         "allMachines matches everything",
		fromVersion:   "1.18.1",
		toVersion:     "1.20.0",
		targets:       targets(upgrades.Controller),
		expectedSteps: []string{"step 1 - 1.20.0", "step 3 - 1.20.0"},
	},
	{
		about:         "state step error aborts, subsequent state steps not run",
		fromVersion:   "1.10.0",
		targets:       targets(upgrades.Controller),
		expectedSteps: []string{"state step 1 - 1.11.0"},
		err:           "state step 2 error: upgrade error occurred",
	},
	{
		about:         "error aborts, subsequent steps not run",
		fromVersion:   "1.11.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.12.0"},
		err:           "step 2 error: upgrade error occurred",
	},
	{
		about:         "default from version is 1.16",
		fromVersion:   "",
		targets:       targets(upgrades.Controller),
		expectedSteps: []string{"step 2 - 1.17.1", "step 2 - 1.18.0"},
	},
	{
		about:         "controllers don't get database master",
		fromVersion:   "1.20.0",
		toVersion:     "1.21.0",
		targets:       targets(upgrades.Controller),
		expectedSteps: []string{"state step 2 - 1.21.0", "step 1 - 1.21.0"},
	},
	{
		about:         "database master only (not actually possible in reality)",
		fromVersion:   "1.20.0",
		toVersion:     "1.21.0",
		targets:       targets(upgrades.DatabaseMaster),
		expectedSteps: []string{"state step 1 - 1.21.0", "step 1 - 1.21.0"},
	},
	{
		about:       "all state steps are run first",
		fromVersion: "1.20.0",
		toVersion:   "1.22.0",
		targets:     targets(upgrades.DatabaseMaster, upgrades.Controller),
		expectedSteps: []string{
			"state step 1 - 1.21.0", "state step 2 - 1.21.0",
			"state step 1 - 1.22.0", "state step 2 - 1.22.0",
			"step 1 - 1.21.0",
			"step 1 - 1.22.0", "step 2 - 1.22.0",
		},
	},
	{
		about:         "machine with multiple targets - each step only run once",
		fromVersion:   "1.20.0",
		toVersion:     "1.21.0",
		targets:       targets(upgrades.HostMachine, upgrades.Controller),
		expectedSteps: []string{"state step 2 - 1.21.0", "step 1 - 1.21.0"},
	},
	{
		about:         "step with multiple targets",
		fromVersion:   "1.21.0",
		toVersion:     "1.22.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.22.0", "step 2 - 1.22.0"},
	},
	{
		about:         "machine and step with multiple targets - each step only run once",
		fromVersion:   "1.21.0",
		toVersion:     "1.22.0",
		targets:       targets(upgrades.HostMachine, upgrades.Controller),
		expectedSteps: []string{"state step 2 - 1.22.0", "step 1 - 1.22.0", "step 2 - 1.22.0"},
	},
	{
		about:         "upgrade to alpha release runs steps for final release",
		fromVersion:   "1.20.0",
		toVersion:     "1.21-alpha1",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.21.0"},
	},
	{
		about:         "upgrade to beta release runs steps for final release",
		fromVersion:   "1.20.0",
		toVersion:     "1.21-beta2",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.21.0"},
	},
	{
		about:         "starting release steps included when upgrading from an alpha release",
		fromVersion:   "1.20-alpha3",
		toVersion:     "1.21.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0", "step 1 - 1.21.0"},
	},
	{
		about:         "starting release steps included when upgrading from an beta release",
		fromVersion:   "1.20-beta1",
		toVersion:     "1.21.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.20.0", "step 2 - 1.20.0", "step 1 - 1.21.0"},
	},
	{
		about:         "nothing happens when the version hasn't changed but contains a tag",
		fromVersion:   "1.21-alpha1",
		toVersion:     "1.21-alpha1",
		targets:       targets(upgrades.DatabaseMaster),
		expectedSteps: []string{},
	},
	{
		about:         "upgrades between pre-final versions should run steps for the final version",
		fromVersion:   "1.21-beta2",
		toVersion:     "1.21-beta3",
		targets:       targets(upgrades.DatabaseMaster),
		expectedSteps: []string{"state step 1 - 1.21.0", "step 1 - 1.21.0"},
	},
}

func (s *upgradeSuite) TestPerformUpgrade(c *gc.C) {
	s.PatchValue(upgrades.StateUpgradeOperations, stateUpgradeOperations)
	s.PatchValue(upgrades.UpgradeOperations, upgradeOperations)
	for i, test := range upgradeTests {
		c.Logf("%d: %s", i, test.about)
		var messages []string
		ctx := &mockContext{
			messages: messages,
			state:    &mockStateBackend{},
		}
		fromVersion := version.Zero
		if test.fromVersion != "" {
			fromVersion = version.MustParse(test.fromVersion)
		}
		toVersion := version.MustParse("1.18.0")
		if test.toVersion != "" {
			toVersion = version.MustParse(test.toVersion)
		}
		s.PatchValue(&jujuversion.Current, toVersion)
		err := upgrades.PerformUpgrade(fromVersion, test.targets, ctx)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(ctx.messages, jc.DeepEquals, test.expectedSteps)
	}
}

type contextStep struct {
	useAPI bool
}

func (s *contextStep) Description() string {
	return "something"
}

func (s *contextStep) Targets() []upgrades.Target {
	return []upgrades.Target{upgrades.Controller}
}

func (s *contextStep) Run(context upgrades.Context) error {
	if s.useAPI {
		context.APIState()
	} else {
		context.State()
	}
	return nil
}

func (s *upgradeSuite) TestStateStepsGetRestrictedContext(c *gc.C) {
	s.PatchValue(upgrades.StateUpgradeOperations, func() []upgrades.Operation {
		return []upgrades.Operation{
			&mockUpgradeOperation{
				targetVersion: version.MustParse("1.21.0"),
				steps:         []upgrades.Step{&contextStep{useAPI: true}},
			},
		}
	})

	s.PatchValue(upgrades.UpgradeOperations,
		func() []upgrades.Operation { return nil })

	s.checkContextRestriction(c, "API not available from this context")
}

func (s *upgradeSuite) TestAPIStepsGetRestrictedContext(c *gc.C) {
	s.PatchValue(upgrades.StateUpgradeOperations,
		func() []upgrades.Operation { return nil })

	s.PatchValue(upgrades.UpgradeOperations, func() []upgrades.Operation {
		return []upgrades.Operation{
			&mockUpgradeOperation{
				targetVersion: version.MustParse("1.21.0"),
				steps:         []upgrades.Step{&contextStep{useAPI: false}},
			},
		}
	})

	s.checkContextRestriction(c, "State not available from this context")
}

func (s *upgradeSuite) checkContextRestriction(c *gc.C, expectedPanic string) {
	fromVersion := version.MustParse("1.20.0")
	type fakeAgentConfigSetter struct{ agent.ConfigSetter }
	ctx := upgrades.NewContext(fakeAgentConfigSetter{}, nil, &mockStateBackend{})
	c.Assert(
		func() { _ = upgrades.PerformUpgrade(fromVersion, targets(upgrades.Controller), ctx) },
		gc.PanicMatches, expectedPanic,
	)
}

func (s *upgradeSuite) TestStateStepsNotAttemptedWhenNoStateTarget(c *gc.C) {
	stateCount := 0
	stateUpgradeOperations := func() []upgrades.Operation {
		stateCount++
		return nil
	}
	s.PatchValue(upgrades.StateUpgradeOperations, stateUpgradeOperations)

	apiCount := 0
	upgradeOperations := func() []upgrades.Operation {
		apiCount++
		return nil
	}
	s.PatchValue(upgrades.UpgradeOperations, upgradeOperations)

	fromVers := version.MustParse("1.18.0")
	state := &mockStateBackend{}
	ctx := &mockContext{state: state}
	check := func(target upgrades.Target, expectedStateCallCount int, expectedStateMethodCalls []string) {
		stateCount = 0
		apiCount = 0
		err := upgrades.PerformUpgrade(fromVers, targets(target), ctx)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(stateCount, gc.Equals, expectedStateCallCount)
		c.Assert(apiCount, gc.Equals, 1)
		state.CheckCallNames(c, expectedStateMethodCalls...)
		state.ResetCalls()
	}

	check(upgrades.Controller, 1, nil)
	check(upgrades.DatabaseMaster, 1, nil)
	check(upgrades.AllMachines, 0, nil)
	check(upgrades.HostMachine, 0, nil)
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

func (s *upgradeSuite) TestStateUpgradeOperationsVersions(c *gc.C) {
	versions := extractUpgradeVersions(c, (*upgrades.StateUpgradeOperations)())
	c.Assert(versions, gc.DeepEquals, []string{"3.6.4", "3.6.5"})
}

func (s *upgradeSuite) TestUpgradeOperationsVersions(c *gc.C) {
	versions := extractUpgradeVersions(c, (*upgrades.UpgradeOperations)())
	c.Assert(versions, gc.DeepEquals, []string(nil))
}

func extractUpgradeVersions(c *gc.C, ops []upgrades.Operation) []string {
	var versions []string
	for _, utv := range ops {
		vers := utv.TargetVersion()
		// Upgrade steps should only be targeted at final versions (not alpha/beta).
		c.Check(vers.Tag, gc.Equals, "")
		versions = append(versions, vers.String())
	}
	return versions
}
