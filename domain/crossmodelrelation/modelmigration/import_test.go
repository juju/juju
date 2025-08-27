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
		// provide a UUID via the migration, it should be used
		// in the new offer.
		OfferUUID:       offerUUID.String(),
		OfferName:       "test",
		Endpoints:       map[string]string{"db-admin": "db-admin"},
		ApplicationName: "test",
	}
	app.AddOffer(offerArgs)
	offerArgs2 := description.ApplicationOfferArgs{
		// No uuid is provided during the migration, one should be
		// created.
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
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.importService.EXPECT().ImportOffers(
		gomock.Any(),
		importOfferArgsMatcher{c: c, expected: input, requiredUUID: offerUUID},
	).Return(nil)

	// Act
	err := s.newImportOperation(c).importOffers(c.Context(), []description.Application{app})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

type importOfferArgsMatcher struct {
	c            *tc.C
	requiredUUID uuid.UUID
	expected     []crossmodelrelation.OfferImport
}

func (m importOfferArgsMatcher) Matches(x interface{}) bool {
	obtained, ok := x.([]crossmodelrelation.OfferImport)
	m.c.Assert(ok, tc.IsTrue)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)
	m.c.Check(obtained, tc.UnorderedMatch[[]crossmodelrelation.OfferImport](mc), m.expected)

	// One of the offerUUIDs must be the expected one.
	var found bool
	for _, o := range obtained {
		if o.UUID == m.requiredUUID {
			found = true
			break
		}
	}
	m.c.Check(found, tc.IsTrue,
		tc.Commentf("input does not contain offer UUID %q: %+v", m.requiredUUID, obtained))
	return true
}

func (m importOfferArgsMatcher) String() string {
	return "match CreateOfferArgs"
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
