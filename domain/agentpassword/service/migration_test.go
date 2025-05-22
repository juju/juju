// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/agentpassword"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	"github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

type migrationServiceSuite struct {
	state *MockMigrationState
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) TestGetAllUnitPasswordHashes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.UnitPasswordHashes{
		"unit/0": "hash",
	}

	s.state.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)

	service := NewMigrationService(s.state)
	result, err := service.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, hashes)
}

func (s *migrationServiceSuite) TestGetAllUnitPasswordHashesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.UnitPasswordHashes{
		"unit/0": "hash",
	}

	s.state.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, errors.Errorf("boom"))

	service := NewMigrationService(s.state)
	_, err := service.GetAllUnitPasswordHashes(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *migrationServiceSuite) TestSetUnitPasswordHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := agentpassword.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, passwordHash).Return(nil)

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(c.Context(), unitName, passwordHash)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestSetUnitPasswordHashNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := agentpassword.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, agentpassworderrors.UnitNotFound)

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(c.Context(), unitName, passwordHash)
	c.Assert(err, tc.ErrorIs, agentpassworderrors.UnitNotFound)
}

func (s *migrationServiceSuite) TestSetUnitPasswordHashSettingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	unitName := unit.Name("unit/0")
	rand, err := internalpassword.RandomPassword()
	c.Assert(err, tc.ErrorIsNil)

	passwordHash := agentpassword.PasswordHash(rand)

	s.state.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().SetUnitPasswordHash(gomock.Any(), unitUUID, passwordHash).Return(errors.Errorf("boom"))

	service := NewMigrationService(s.state)
	err = service.SetUnitPasswordHash(c.Context(), unitName, passwordHash)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}
