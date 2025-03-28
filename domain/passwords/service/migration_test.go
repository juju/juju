// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
)

type migrationServiceSuite struct {
	state *MockMigrationState
}

var _ = gc.Suite(&migrationServiceSuite{})

func (s *migrationServiceSuite) TestGetAllUnitPasswordHashes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]passwords.PasswordHash{
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

func (s *migrationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}
