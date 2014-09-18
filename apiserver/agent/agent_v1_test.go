package agent_test

import (
	gc "launchpad.net/gocheck"

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
	testAgentFailsWithNonAgentV0(c, &s.baseSuite, factoryWrapperV1)
}

func (s *agentSuiteV1) TestAgentSucceedsWithUnitAgent(c *gc.C) {
	testAgentSucceedsWithUnitAgentV0(c, &s.baseSuite, factoryWrapperV1)
}

func (s *agentSuiteV1) TestGetEntities(c *gc.C) {
	testGetEntitiesV0(c, &s.baseSuite, s.newAPI(c))
}

func (s *agentSuiteV1) TestGetEntitiesContainer(c *gc.C) {
	auth := s.authorizer
	auth.Tag = s.container.Tag()
	api, err := agent.NewAgentAPIV1(s.State, s.resources, auth)
	c.Assert(err, gc.IsNil)
	testGetEntitiesContainerV0(c, &s.baseSuite, api)
}

func (s *agentSuiteV1) TestGetEntitiesNotFound(c *gc.C) {
	testGetEntitiesNotFoundV0(c, &s.baseSuite, s.newAPI(c))
}

func (s *agentSuiteV1) TestSetPasswords(c *gc.C) {
	testSetPasswordsV0(c, &s.baseSuite, s.newAPI(c))
}

func (s *agentSuiteV1) TestSetPasswordsShort(c *gc.C) {
	testSetPasswordsShortV0(c, &s.baseSuite, s.newAPI(c))
}

func (s *agentSuiteV1) newAPI(c *gc.C) *agent.AgentAPIV1 {
	api, err := agent.NewAgentAPIV1(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return api
}
