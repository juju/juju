// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var _ Observer = (*AgentPresence)(nil)

type AgentPresenceSuite struct {
	testing.IsolationSuite

	domainServicesGetter *MockDomainServicesGetter
	modelService         *MockModelService
	statusService        *MockStatusService
}

var _ = tc.Suite(&AgentPresenceSuite{})

func (s *AgentPresenceSuite) TestLoginForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)

	// TODO (stickupkid): Once the machine domain is done, this should set
	// the machine presence.

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLeaveForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(2)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)
	s.statusService.EXPECT().DeleteUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) TestLeaveForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	observer := s.newObserver(c)
	observer.Login(context.Background(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) TestLeaveWithoutLogin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	observer := s.newObserver(c)
	observer.Leave(context.Background())
}

func (s *AgentPresenceSuite) newObserver(c *tc.C) *AgentPresence {
	return NewAgentPresence(AgentPresenceConfig{
		DomainServicesGetter: s.domainServicesGetter,
		Logger:               loggertesting.WrapCheckLog(c),
	})
}

func (s *AgentPresenceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	return ctrl
}
