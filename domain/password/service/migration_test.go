// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/password"
	"github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

type migrationServiceSuite struct {
	state *MockMigrationState
}

var _ = gc.Suite(&migrationServiceSuite{})

func (s *migrationServiceSuite) TestGetAllUnitPasswordHashes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]password.PasswordHash{
		"foo": {
			"unit/0": "hash",
		},
	}

	s.state.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)

	service := NewMigrationService(s.state)
	result, err := service.GetAllUnitPasswordHashes(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, hashes)
}

func (s *migrationServiceSuite) TestGetAllUnitPasswordHashesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]password.PasswordHash{
		"foo": {
			"unit/0": "hash",
		},
	}

	s.state.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, errors.Errorf("boom"))

	service := NewMigrationService(s.state)
	_, err := service.GetAllUnitPasswordHashes(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *migrationServiceSuite) TestSetUnitPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := password.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, passwordHash).Return(nil)

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(context.Background(), unitName, passwordHash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestSetUnitPasswordHashNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := password.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, applicationerrors.UnitNotFound)

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(context.Background(), unitName, passwordHash)
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *migrationServiceSuite) TestSetUnitPasswordHashSettingError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)

	passwordHash := password.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, passwordHash).Return(errors.Errorf("boom"))

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(context.Background(), unitName, passwordHash)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *migrationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}
