// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type AgentPresenceSuite struct {
	testing.IsolationSuite

	domainServiceGetter *MockDomainServiceGetter
	modelService        *MockModelService
	applicationService  *MockApplicationService
}

var _ = gc.Suite(&AgentPresenceSuite{})

func (s *AgentPresenceSuite) TestLogin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	//observer := s.newObserver(c)
	//observer.Login(context.Background(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), false, "user data")
}

func (s *AgentPresenceSuite) newObserver(c *gc.C) *AgentPresence {
	return NewAgentPresence(AgentPresenceConfig{
		DomainServiceGetter: s.domainServiceGetter,
		Logger:              loggertesting.WrapCheckLog(c),
	})
}

func (s *AgentPresenceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServiceGetter = NewMockDomainServiceGetter(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	return ctrl
}
