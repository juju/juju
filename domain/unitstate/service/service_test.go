// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/errors"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
)

type serviceSuite struct {
	st *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSetStateAllAttributes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	uuid := unittesting.GenUnitUUID(c)

	exp := s.st.EXPECT()
	exp.GetUnitUUIDForName(gomock.Any(), name).Return(uuid, nil)
	exp.EnsureUnitStateRecord(gomock.Any(), uuid).Return(nil)
	exp.UpdateUnitStateUniter(gomock.Any(), uuid, "some-uniter-state-yaml").Return(nil)
	exp.UpdateUnitStateStorage(gomock.Any(), uuid, "some-storage-state-yaml").Return(nil)
	exp.UpdateUnitStateSecret(gomock.Any(), uuid, "some-secret-state-yaml").Return(nil)
	exp.SetUnitStateCharm(gomock.Any(), uuid, map[string]string{"one-key": "one-value"}).Return(nil)
	exp.SetUnitStateRelation(gomock.Any(), uuid, map[int]string{1: "one-value"}).Return(nil)

	err := NewService(s.st).SetState(context.Background(), unitstate.UnitState{
		Name:          name,
		CharmState:    ptr(map[string]string{"one-key": "one-value"}),
		UniterState:   ptr("some-uniter-state-yaml"),
		RelationState: ptr(map[int]string{1: "one-value"}),
		StorageState:  ptr("some-storage-state-yaml"),
		SecretState:   ptr("some-secret-state-yaml"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetStateSubsetAttributes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	uuid := unittesting.GenUnitUUID(c)

	exp := s.st.EXPECT()
	exp.GetUnitUUIDForName(gomock.Any(), name).Return(uuid, nil)
	exp.EnsureUnitStateRecord(gomock.Any(), uuid).Return(nil)
	exp.UpdateUnitStateUniter(gomock.Any(), uuid, "some-uniter-state-yaml").Return(nil)

	err := NewService(s.st).SetState(context.Background(), unitstate.UnitState{
		Name:        name,
		UniterState: ptr("some-uniter-state-yaml"),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetStateUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")

	exp := s.st.EXPECT()
	exp.GetUnitUUIDForName(gomock.Any(), name).Return("", errors.UnitNotFound)

	err := NewService(s.st).SetState(context.Background(), unitstate.UnitState{
		Name:        name,
		UniterState: ptr("some-uniter-state-yaml"),
	})
	c.Check(err, jc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) TestGetState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name)

	_, err := NewService(s.st).GetState(context.Background(), name)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetStateUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	name := unittesting.GenNewName(c, "unit/0")
	s.st.EXPECT().GetUnitState(gomock.Any(), name).Return(unitstate.RetrievedUnitState{}, unitstateerrors.UnitNotFound)

	_, err := NewService(s.st).GetState(context.Background(), name)
	c.Assert(err, jc.ErrorIs, unitstateerrors.UnitNotFound)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	s.st.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	return ctrl
}
