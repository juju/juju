// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
)

type serviceSuite struct {
	st *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")

	as := unitstate.UnitState{
		Name:          name,
		CharmState:    ptr(map[string]string{"one-key": "one-value"}),
		UniterState:   ptr("some-uniter-state-yaml"),
		RelationState: ptr(map[int]string{1: "one-value"}),
		StorageState:  ptr("some-storage-state-yaml"),
		SecretState:   ptr("some-secret-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as)

	err := NewService(s.st).SetState(c.Context(), as)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")

	as := unitstate.UnitState{
		Name:        name,
		UniterState: ptr("some-uniter-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as).Return(errors.UnitNotFound)

	err := NewService(s.st).SetState(c.Context(), as)
	c.Check(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name)

	_, err := NewService(s.st).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name).Return(unitstate.RetrievedUnitState{}, unitstateerrors.UnitNotFound)

	_, err := NewService(s.st).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	return ctrl
}
