// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationSuite struct {
	testhelpers.IsolationSuite

	state *MockMigrationState
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestMigrationService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)

	s.state.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), modelUUID).Return(map[string]string{
		"foo": "bar",
	}, nil)

	leadershipService := NewMigrationService(s.state)
	leaders, err := leadershipService.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leaders, tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *migrationSuite) TestMigrationServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.GenUUID(c)

	s.state.EXPECT().GetApplicationLeadershipForModel(gomock.Any(), modelUUID).Return(map[string]string{
		"foo": "bar",
	}, errors.Errorf("boom"))

	leadershipService := NewMigrationService(s.state)
	_, err := leadershipService.GetApplicationLeadershipForModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorMatches, "boom")

}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}
