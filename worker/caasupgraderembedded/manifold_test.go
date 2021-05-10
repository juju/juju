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
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/caasupgraderembedded"
	"github.com/juju/juju/worker/caasupgraderembedded/mocks"
)

type manifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	context  dependency.Context

	apiCaller *mocks.MockAPICaller
	client    *mocks.MockUpgraderClient
	agent     *mocks.MockAgent

	previousAgentVersion version.Number
	agentName            string

	ctrl *gomock.Controller
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.agentName = names.NewUnitTag("snappass/0").Id()

	var err error
	s.previousAgentVersion, err = version.Parse("2.9.0")
	c.Assert(err, jc.ErrorIsNil)

	s.ctrl = gomock.NewController(c)
	s.apiCaller = mocks.NewMockAPICaller(s.ctrl)
	s.client = mocks.NewMockUpgraderClient(s.ctrl)
	s.agent = mocks.NewMockAgent(s.ctrl)

	s.context = s.newContext(nil)
	s.manifold = caasupgraderembedded.Manifold(s.validConfig())
}

func (s *manifoldSuite) validConfig() caasupgraderembedded.ManifoldConfig {
	return caasupgraderembedded.ManifoldConfig{
		AgentName:            s.agentName,
		APICallerName:        "api-caller",
		UpgradeStepsGateName: "upgrade-steps-gate",
		PreviousAgentVersion: s.previousAgentVersion,
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
		"api-caller": s.apiCaller,
		s.agentName:  s.agent,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) TestMissingAgentName(c *gc.C) {
	config := s.validConfig()
	config.AgentName = ""
	s.checkConfigInvalid(c, config, "empty AgentName not valid")
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	config := s.validConfig()
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingUpgradeStepsGateName(c *gc.C) {
	config := s.validConfig()
	config.UpgradeStepsGateName = ""
	s.checkConfigInvalid(c, config, "empty UpgradeStepsGateName not valid")
}
func (s *manifoldSuite) TestMissingPreviousAgentVersion(c *gc.C) {
	config := s.validConfig()
	config.PreviousAgentVersion = version.Zero
	s.checkConfigInvalid(c, config, "previous agent version not specified not valid")
}
func (s *manifoldSuite) TestMissingNewClient(c *gc.C) {
	config := s.validConfig()
	config.NewClient = nil
	s.checkConfigInvalid(c, config, "nil NewClient not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *gc.C, config caasupgraderembedded.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

var expectedInputs = []string{"snappass/0", "api-caller", "upgrade-steps-gate"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewClient")
	s.CheckCall(c, 0, "NewClient", s.apiCaller)
}
