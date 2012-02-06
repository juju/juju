package agent_test

import (
	. "launchpad.net/gocheck"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

func (s *AgentSuite) TestFails(c *C) {
	c.Fail()
}
