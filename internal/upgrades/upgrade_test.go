// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/mongo"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/version"
)

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
}

func (c *mockContext) APIState() base.APICaller {
	return c.apiState
}

func (c *mockContext) AgentConfig() agent.ConfigSetter {
	if c.realAgentConfig != nil {
		return c.realAgentConfig
	}
	return c.agentConfig
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
		about:         "step with multiple targets",
		fromVersion:   "1.21.0",
		toVersion:     "1.22.0",
		targets:       targets(upgrades.HostMachine),
		expectedSteps: []string{"step 1 - 1.22.0", "step 2 - 1.22.0"},
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
}

func (s *upgradeSuite) TestPerformUpgradeSteps(c *gc.C) {
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
		s.PatchValue(&jujuversion.Current, toVersion)
		err := upgrades.PerformUpgradeSteps(fromVersion, test.targets, ctx)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
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

func (s *upgradeSuite) TestUpgradeOperationsVersions(c *gc.C) {
	versions := extractUpgradeVersions(c, (*upgrades.UpgradeOperations)())
	c.Assert(versions, gc.DeepEquals, []string{"6.6.6"})
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
