package agent_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// V1 test suite, no additional or changed tests.

func factoryWrapperV1(st *state.State, resources *common.Resources, auth common.Authorizer) (interface{}, error) {
	return agent.NewAgentAPIV1(st, resources, auth)
}

type agentSuiteV1 struct {
	baseSuite
}

var _ = gc.Suite(&agentSuiteV1{})

func (s *agentSuiteV1) TestAgentFailsWithNonAgent(c *gc.C) {
	s.testAgentFailsWithNonAgentV0(c, factoryWrapperV1)
}

func (s *agentSuiteV1) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	s.testAgentSucceedsWithUnitAgentV0(c, factoryWrapperV1)
}

func (s *agentSuiteV1) TestGetEntities(c *gc.C) {
	s.testGetEntitiesV0(c, s.newAPI(c))
}

func (s *agentSuiteV1) TestGetEntitiesContainer(c *gc.C) {
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	api, err := agent.NewAgentAPIV1(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)
	s.testGetEntitiesContainerV0(c, api)
}

func (s *agentSuiteV1) TestGetEntitiesNotFound(c *gc.C) {
	s.testGetEntitiesNotFoundV0(c, s.newAPI(c))
}

func (s *agentSuiteV1) TestSetPasswords(c *gc.C) {
	s.testSetPasswordsV0(c, s.newAPI(c))
}

func (s *agentSuiteV1) TestSetPasswordsShort(c *gc.C) {
	s.testSetPasswordsShortV0(c, s.newAPI(c))
}

func (s *agentSuiteV1) newAPI(c *gc.C) *agent.AgentAPIV1 {
	api, err := agent.NewAgentAPIV1(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return api
}
