// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	removal "github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

type machineSuite struct {
	baseSuite
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) TestRemoveMachineNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), mUUID.String()).Return([]string{"some-unit-id"}, []string{"some-machine-id"}, nil)
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the machine or unit expectations here, so
	// we'll assume that the machine/unit no longer exists, to prevent this test
	// from depending on the machine/unit removal logic.
	exp.MachineExists(gomock.Any(), "some-machine-id").Return(false, nil)
	exp.UnitExists(gomock.Any(), "some-unit-id").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), mUUID.String()).Return(nil, nil, nil)
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), mUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), mUUID.String()).Return(nil, nil, nil)

	// The first normal removal scheduled immediately.
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), mUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), mUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().MachineExists(gomock.Any(), mUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveMachine(c.Context(), mUUID, false, 0)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestMarkMachineAsDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(true, nil)
	exp.MarkMachineAsDead(gomock.Any(), mUUID.String()).Return(nil)

	err := s.newService(c).MarkMachineAsDead(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineSuite) TestMarkMachineAsDeadMachineDoesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(false, nil)

	err := s.newService(c).MarkMachineAsDead(c.Context(), mUUID)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *machineSuite) TestMarkMachineAsDeadError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mUUID := machinetesting.GenUUID(c)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), mUUID.String()).Return(false, errors.Errorf("the front fell off"))

	err := s.newService(c).MarkMachineAsDead(c.Context(), mUUID)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *machineSuite) TestProcessRemovalJobInvalidJobType(c *tc.C) {
	var invalidJobType removal.JobType = 500

	job := removal.Job{
		RemovalType: invalidJobType,
	}

	err := s.newService(c).processMachineRemovalJob(c.Context(), job)
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobTypeNotValid)
}

func (s *machineSuite) TestExecuteJobForMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(-1, machineerrors.MachineNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineSuite) TestExecuteJobForMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(-1, errors.Errorf("the front fell off"))

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorMatches, ".*the front fell off")
}

func (s *machineSuite) TestExecuteJobForMachineStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *machineSuite) TestExecuteJobForMachineInstanceDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetInstanceLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineSuite) TestExecuteJobForMachineDyingReleaseAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetInstanceLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetMachineNetworkInterfaces(gomock.Any(), j.EntityUUID).Return(nil, machineerrors.MachineNotFound)
	exp.DeleteMachine(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineSuite) TestExecuteJobForMachineDyingReleaseAddressesNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetInstanceLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetMachineNetworkInterfaces(gomock.Any(), j.EntityUUID).Return([]string{"foo"}, nil)
	exp.DeleteMachine(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	s.provider.EXPECT().ReleaseContainerAddresses(gomock.Any(), []string{"foo"}).Return(coreerrors.NotSupported)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *machineSuite) TestExecuteJobForMachineDyingDeleteMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newMachineJob(c)

	exp := s.state.EXPECT()
	exp.GetMachineLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
	exp.GetInstanceLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetMachineNetworkInterfaces(gomock.Any(), j.EntityUUID).Return([]string{"foo"}, nil)
	exp.DeleteMachine(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	s.provider.EXPECT().ReleaseContainerAddresses(gomock.Any(), []string{"foo"}).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newMachineJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.MachineJob,
		EntityUUID:  machinetesting.GenUUID(c).String(),
	}
}
