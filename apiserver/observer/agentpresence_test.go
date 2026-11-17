// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
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

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(status.PresenceToken("token"), nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetMachinePresence(gomock.Any(), machine.Name("0")).Return(status.PresenceToken("token"), nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLoginFromController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, true, "user data")
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLoginForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUserTag("bob"), names.NewModelTag("bar"), uuid, false, "user data")
}

func (s *AgentPresenceSuite) TestLeaveForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(2)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(status.PresenceToken("token"), nil)
	s.statusService.EXPECT().DeleteUnitPresenceWithToken(gomock.Any(), unit.Name("foo/666"), status.PresenceToken("token")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

// TestLeaveOfOlderConnectionForUnit covers an agent that logs in on a new
// connection before its old one is closed. Each connection must only ask to
// remove the presence it recorded itself, so the old one leaving does not
// remove the presence recorded by the new one.
func (s *AgentPresenceSuite) TestLeaveOfOlderConnectionForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil).Times(2)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(3)
	gomock.InOrder(
		s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(status.PresenceToken("old-token"), nil),
		s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(status.PresenceToken("new-token"), nil),
		s.statusService.EXPECT().DeleteUnitPresenceWithToken(gomock.Any(), unit.Name("foo/666"), status.PresenceToken("old-token")).Return(nil),
	)

	oldConn := s.newObserver(c)
	oldConn.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	newConn := s.newObserver(c)
	newConn.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	oldConn.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveAfterFailedLoginForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetUnitPresence(gomock.Any(), unit.Name("foo/666")).Return(status.PresenceToken(""), errors.New("boom"))
	// No presence was recorded, so there is nothing to delete.

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewUnitTag("foo/666"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(2)
	s.statusService.EXPECT().SetMachinePresence(gomock.Any(), machine.Name("0")).Return(status.PresenceToken("token"), nil)
	s.statusService.EXPECT().DeleteMachinePresenceWithToken(gomock.Any(), machine.Name("0"), status.PresenceToken("token")).Return(nil)

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

// TestLeaveOfOlderConnectionForMachine is the machine variant of
// TestLeaveOfOlderConnectionForUnit.
func (s *AgentPresenceSuite) TestLeaveOfOlderConnectionForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil).Times(2)
	s.modelService.EXPECT().StatusService().Return(s.statusService).Times(3)
	gomock.InOrder(
		s.statusService.EXPECT().SetMachinePresence(gomock.Any(), machine.Name("0")).Return(status.PresenceToken("old-token"), nil),
		s.statusService.EXPECT().SetMachinePresence(gomock.Any(), machine.Name("0")).Return(status.PresenceToken("new-token"), nil),
		s.statusService.EXPECT().DeleteMachinePresenceWithToken(gomock.Any(), machine.Name("0"), status.PresenceToken("old-token")).Return(nil),
	)

	oldConn := s.newObserver(c)
	oldConn.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
	newConn := s.newObserver(c)
	newConn.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
	oldConn.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveAfterFailedLoginForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), uuid).Return(s.modelService, nil)
	s.modelService.EXPECT().StatusService().Return(s.statusService)
	s.statusService.EXPECT().SetMachinePresence(gomock.Any(), machine.Name("0")).Return(status.PresenceToken(""), errors.New("boom"))
	// No presence was recorded, so there is nothing to delete.

	observer := s.newObserver(c)
	observer.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, false, "user data")
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveFromController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

	observer := s.newObserver(c)
	// Exercise the departure guard independently of the presence login guard.
	observer.BaseObserver.Login(c.Context(), names.NewMachineTag("0"), names.NewModelTag("bar"), uuid, true, "user data")
	observer.modelService = s.modelService
	observer.Leave(c.Context())
}

func (s *AgentPresenceSuite) TestLeaveForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must0(c, coremodel.NewUUID)

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
