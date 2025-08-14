// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

var _ Observer = (*AgentPresence)(nil)

type AgentPresenceSuite struct {
	testhelpers.IsolationSuite

	domainServicesGetter *MockDomainServicesGetter
	modelService         *MockModelService
	statusService        *MockStatusService
}

func TestAgentPresenceSuite(t *testing.T) {
	tc.Run(t, &AgentPresenceSuite{})
}

func (s *AgentPresenceSuite) TestLoginForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := coremodel.GenUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := coremodel.GenUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)

	// TODO (stickupkid): Once the machine domain is done, this should set
	// the machine presence.

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := coremodel.GenUUID(c)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLeaveForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := coremodel.GenUUID(c)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(2)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)
	s.statusService.EXPECT().DeleteUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := coremodel.GenUUID(c)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveWithoutLogin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	observer := s.newObserver(c)
	observer.Leave(c.Context())
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
