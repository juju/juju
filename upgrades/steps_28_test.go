// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apicallermocks "github.com/juju/juju/api/base/mocks"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/upgrades/mocks"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	configsettermocks "github.com/juju/juju/worker/upgradedatabase/mocks"
)

var v280 = version.MustParse("2.8.0")

type steps28Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps28Suite{})

func (s *steps28Suite) TestIncrementTasksSequence(c *gc.C) {
	step := findStateStep(c, v280, "increment tasks sequence by 1")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps28Suite) TestAddMachineIDToSubordinates(c *gc.C) {
	step := findStateStep(c, v280, "add machine ID to subordinate units")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps28Suite) TestPopulateRebootHandledFlagsForDeployedUnits(c *gc.C) {
	step := findStep(c, v280, "ensure currently running units do not fire start hooks thinking a reboot has occurred")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
}

func (s *steps28Suite) TestMoveUniterStateToControllerStep(c *gc.C) {
	step := findStep(c, v280, "write uniter state to controller for all running units and remove files")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
}

type mockSteps28Suite struct {
	testing.BaseSuite

	dataDir string
	tagOne  names.Tag
	tagTwo  names.Tag

	opStateOne         operation.State
	opStateTwo         operation.State
	opStateOneYaml     string
	opStateTwoYaml     string
	opStateOneFileName string
	opStateTwoFileName string

	mockCtx         *mocks.MockContext
	mockClient      *mocks.MockUpgradeStepsClient
	mockAgentConfig *configsettermocks.MockConfigSetter
	mockAPICaller   *apicallermocks.MockAPICaller
}

var _ = gc.Suite(&mockSteps28Suite{})

func (s *mockSteps28Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.tagOne = names.NewUnitTag("testing/0")
	s.tagTwo = names.NewUnitTag("testing/1")

	s.opStateOne = operation.State{
		Leader: true,
		Kind:   operation.Continue,
		Step:   operation.Pending,
	}

	s.opStateTwo = operation.State{
		Kind: operation.RunHook,
		Step: operation.Pending,
		Hook: &hook.Info{Kind: hooks.ConfigChanged},
	}

	s.dataDir = c.MkDir()
	agentDir := agent.BaseDir(s.dataDir)
	unitOneStateDir := filepath.Join(agentDir, s.tagOne.String())
	s.opStateOneYaml, s.opStateOneFileName = writeUnitStateFile(c, unitOneStateDir, s.opStateOne)
	unitTwoStateDir := filepath.Join(agentDir, s.tagTwo.String())
	s.opStateTwoYaml, s.opStateTwoFileName = writeUnitStateFile(c, unitTwoStateDir, s.opStateTwo)
}

// writeUnitStateFile writes the operation.State in yaml format to the
// path/uniter/state file.  It returns the yaml in string form and the
// full path to the file written.
func writeUnitStateFile(c *gc.C, path string, st operation.State) (string, string) {
	dir := filepath.Join(path, "state")
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	filePath := filepath.Join(dir, "uniter")

	err = st.Validate()
	c.Assert(err, jc.ErrorIsNil)
	content, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(filePath, content, 0644)
	c.Assert(err, jc.ErrorIsNil)

	return string(content), filePath
}

func (s *mockSteps28Suite) TestMoveUniterStateToControllerIAAS(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAPIState()
	s.expectAgentConfigValueIAAS()
	s.expectWriteTwoUniterState()
	s.patchClient()

	err := upgrades.MoveUniterStateToController(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(s.opStateOneFileName)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	_, err = os.Stat(s.opStateTwoFileName)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// Check idempotent
	err = upgrades.MoveUniterStateToController(s.mockCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mockSteps28Suite) TestMoveUniterStateToControllerCAASDoesNothing(c *gc.C) {
	// TODO: (hml) 27-03-2020
	// remove when uniterstate moved for CAAS units and/or relations etc
	// added to move.
	defer s.setup(c).Finish()
	s.expectAgentConfigValueCAAS()
	s.patchClient()

	// Check idempotent
	for i := 0; i < 2; i += 1 {
		c.Logf("round %d", i)
		err := upgrades.MoveUniterStateToController(s.mockCtx)
		c.Assert(err, jc.ErrorIsNil)
		_, err = os.Stat(s.opStateOneFileName)
		c.Assert(err, jc.ErrorIsNil)
		_, err = os.Stat(s.opStateTwoFileName)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *mockSteps28Suite) setup(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)

	s.mockCtx = mocks.NewMockContext(ctlr)
	s.mockAPICaller = apicallermocks.NewMockAPICaller(ctlr)
	s.mockAgentConfig = configsettermocks.NewMockConfigSetter(ctlr)
	s.mockClient = mocks.NewMockUpgradeStepsClient(ctlr)

	s.expectAgentConfig()
	s.expectDataDir()

	return ctlr
}

func (s *mockSteps28Suite) expectAgentConfig() {
	s.mockCtx.EXPECT().AgentConfig().Return(s.mockAgentConfig).AnyTimes()
}

func (s *mockSteps28Suite) expectAPIState() {
	s.mockCtx.EXPECT().APIState().Return(s.mockAPICaller)
}

func (s *mockSteps28Suite) expectDataDir() {
	s.mockAgentConfig.EXPECT().DataDir().Return(s.dataDir).AnyTimes()
}

func (s *mockSteps28Suite) expectAgentConfigValueCAAS() {
	s.mockAgentConfig.EXPECT().Value(agent.ProviderType).Return(k8sprovider.CAASProviderType).AnyTimes()
}

func (s *mockSteps28Suite) expectAgentConfigValueIAAS() {
	s.mockAgentConfig.EXPECT().Value(agent.ProviderType).Return("IAAS").AnyTimes()
}

func (s *mockSteps28Suite) patchClient() {
	s.PatchValue(upgrades.GetUpgradeStepsClient, func(_ base.APICaller) upgrades.UpgradeStepsClient {
		return s.mockClient
	})
}

func (s *mockSteps28Suite) expectWriteTwoUniterState() {
	writeMap := map[names.Tag]string{
		s.tagOne: s.opStateOneYaml,
		s.tagTwo: s.opStateTwoYaml,
	}
	cExp := s.mockClient.EXPECT()
	cExp.WriteUniterState(writeMap).Return(nil)
}
