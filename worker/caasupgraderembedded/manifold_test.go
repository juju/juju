// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgraderembedded_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	coreos "github.com/juju/juju/core/os"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/caasupgraderembedded"
	"github.com/juju/juju/worker/caasupgraderembedded/mocks"
	"github.com/juju/juju/worker/gate"
)

type manifoldSuite struct {
	testing.IsolationSuite
	testing.Stub

	apiCaller   *mocks.MockAPICaller
	client      *mocks.MockUpgraderClient
	agent       *mocks.MockAgent
	agentConfig *mocks.MockConfig

	agentTag             names.UnitTag
	upgradeStepsGateName gate.Lock
	confVersion          version.Number
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.agentTag = names.NewUnitTag("snappass/0")
	s.upgradeStepsGateName = gate.NewLock()

	var err error
	s.confVersion, err = version.Parse("2.9.0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldSuite) setUpCtrl(c *gc.C) func() {
	ctrl := gomock.NewController(c)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)
	s.client = mocks.NewMockUpgraderClient(ctrl)
	s.agent = mocks.NewMockAgent(ctrl)
	s.agentConfig = mocks.NewMockConfig(ctrl)
	return ctrl.Finish
}

func (s *manifoldSuite) validConfig() caasupgraderembedded.ManifoldConfig {
	return caasupgraderembedded.ManifoldConfig{
		AgentName:            s.agentTag.Id(),
		APICallerName:        "api-caller",
		UpgradeStepsGateName: "upgrade-steps-gate",
		NewClient:            s.newClient,
		Logger:               loggo.GetLogger("test"),
	}
}

func (s *manifoldSuite) newClient(apiCaller base.APICaller) caasupgraderembedded.UpgraderClient {
	s.MethodCall(s, "NewClient", apiCaller)
	return s.client
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"api-caller":         s.apiCaller,
		s.agentTag.Id():      s.agent,
		"upgrade-steps-gate": s.upgradeStepsGateName,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) patchVersion(v version.Binary) {
	s.PatchValue(&arch.HostArch, func() string { return v.Arch })
	s.PatchValue(&coreos.HostOS, func() coreos.OSType { return coreos.Ubuntu })
	s.PatchValue(&jujuversion.Current, v.Number)
}

func (s *manifoldSuite) TestMissingAgentName(c *gc.C) {
	defer s.setUpCtrl(c)()

	config := s.validConfig()
	config.AgentName = ""
	s.checkConfigInvalid(c, config, "empty AgentName not valid")
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	defer s.setUpCtrl(c)()

	config := s.validConfig()
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingUpgradeStepsGateName(c *gc.C) {
	defer s.setUpCtrl(c)()

	config := s.validConfig()
	config.UpgradeStepsGateName = ""
	s.checkConfigInvalid(c, config, "empty UpgradeStepsGateName not valid")
}

func (s *manifoldSuite) TestMissingNewClient(c *gc.C) {
	defer s.setUpCtrl(c)()

	config := s.validConfig()
	config.NewClient = nil
	s.checkConfigInvalid(c, config, "nil NewClient not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	defer s.setUpCtrl(c)()

	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *gc.C, config caasupgraderembedded.ManifoldConfig, expect string) {
	defer s.setUpCtrl(c)()

	err := config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

var expectedInputs = []string{
	"snappass/0",
	"api-caller",
	"upgrade-steps-gate",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	defer s.setUpCtrl(c)()

	manifold := caasupgraderembedded.Manifold(s.validConfig())
	c.Assert(manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	defer s.setUpCtrl(c)()

	gomock.InOrder(
		s.agent.EXPECT().CurrentConfig().AnyTimes().Return(s.agentConfig),
	)

	manifold := caasupgraderembedded.Manifold(s.validConfig())
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setUpCtrl(c)()

	s.patchVersion(
		version.Binary{
			Number:  s.confVersion,
			Arch:    "amd64",
			Release: "ubuntu",
		},
	)

	gomock.InOrder(
		s.agent.EXPECT().CurrentConfig().Return(s.agentConfig),
		s.agentConfig.EXPECT().Tag().Return(s.agentTag),
		s.client.EXPECT().SetVersion(s.agentTag.String(), caasupgraderembedded.ToBinaryVersion(s.confVersion, "ubuntu")).Return(nil),
	)

	manifold := caasupgraderembedded.Manifold(s.validConfig())
	w, err := manifold.Start(s.newContext(nil))
	c.Assert(err, jc.ErrorIsNil)

	s.CheckCallNames(c, "NewClient")
	s.CheckCall(c, 0, "NewClient", s.apiCaller)
	workertest.CleanKill(c, w)
}
