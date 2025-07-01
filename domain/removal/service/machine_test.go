// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	machinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	removal "github.com/juju/juju/domain/removal"
)

type machineSuite struct {
	baseSuite
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

func (s *machineSuite) TestRemoveMachineNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), uUUID.String()).Return([]string{"some-unit-id"}, []string{"some-machine-id"}, nil)
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)

	// We don't want to create all the machine or unit expectations here, so
	// we'll assume that the machine/unit no longer exists, to prevent this test
	// from depending on the machine/unit removal logic.
	exp.MachineExists(gomock.Any(), "some-machine-id").Return(false, nil)
	exp.UnitExists(gomock.Any(), "some-unit-id").Return(false, nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), uUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), uUUID.String()).Return(nil, nil, nil)
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), uUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := machinetesting.GenUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.state.EXPECT()
	exp.MachineExists(gomock.Any(), uUUID.String()).Return(true, nil)
	exp.EnsureMachineNotAliveCascade(gomock.Any(), uUUID.String()).Return(nil, nil, nil)

	// The first normal removal scheduled immediately.
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.MachineScheduleRemoval(gomock.Any(), gomock.Any(), uUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveMachine(c.Context(), uUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *machineSuite) TestRemoveMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().MachineExists(gomock.Any(), uUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveMachine(c.Context(), uUUID, false, 0)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
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
