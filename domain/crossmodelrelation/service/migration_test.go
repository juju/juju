// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	modelMigrationState *MockModelMigrationState
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestImportOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "test",
			ApplicationName: "test",
			Endpoints:       []string{"db-admin"},
		}, {
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(nil)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportOffersFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(applicationerrors.ApplicationNotFound)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) service(c *tc.C) *MigrationService {
	return &MigrationService{
		modelState: s.modelMigrationState,
		logger:     loggertesting.WrapCheckLog(c),
	}
}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelMigrationState = NewMockModelMigrationState(ctrl)

	c.Cleanup(func() {
		s.modelMigrationState = nil
	})
	return ctrl
}
