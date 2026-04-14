// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/unitstate"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	st *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestSetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0").String()

	as := unitstate.UnitState{
		Name:          name,
		CharmState:    new(map[string]string{"one-key": "one-value"}),
		UniterState:   new("some-uniter-state-yaml"),
		RelationState: new(map[int]string{1: "one-value"}),
		StorageState:  new("some-storage-state-yaml"),
		SecretState:   new("some-secret-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as)

	err := NewService(s.st, loggertesting.WrapCheckLog(c)).SetState(c.Context(), as)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0").String()

	as := unitstate.UnitState{
		Name:        name,
		UniterState: new("some-uniter-state-yaml"),
	}

	exp := s.st.EXPECT()
	exp.SetUnitState(gomock.Any(), as).Return(applicationerrors.UnitNotFound)

	err := NewService(s.st, loggertesting.WrapCheckLog(c)).SetState(c.Context(), as)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name.String())

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetStateUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name.String()).Return(
		unitstate.RetrievedUnitState{}, applicationerrors.UnitNotFound)

	_, err := NewService(s.st, loggertesting.WrapCheckLog(c)).GetState(c.Context(), name)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)

	c.Cleanup(func() { s.st = nil })

	return ctrl
}
