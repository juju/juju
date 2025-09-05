// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/crossmodelrelation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	importService *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{})
	offerUUID := uuid.MustNewUUID()
	offerArgs := description.ApplicationOfferArgs{
		OfferUUID:       offerUUID.String(),
		OfferName:       "test",
		Endpoints:       map[string]string{"db-admin": "db-admin"},
		ApplicationName: "test",
	}
	app.AddOffer(offerArgs)
	offerUUID2 := uuid.MustNewUUID()
	offerArgs2 := description.ApplicationOfferArgs{
		OfferUUID:       offerUUID2.String(),
		OfferName:       "second",
		Endpoints:       map[string]string{"identity": "identity"},
		ApplicationName: "apple",
	}
	app.AddOffer(offerArgs2)
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            offerUUID,
			Name:            "test",
			ApplicationName: "test",
			Endpoints:       []string{"db-admin"},
		}, {
			UUID:            offerUUID2,
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.importService.EXPECT().ImportOffers(
		gomock.Any(),
		input,
	).Return(nil)

	// Act
	err := s.newImportOperation(c).importOffers(c.Context(), []description.Application{app})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}
