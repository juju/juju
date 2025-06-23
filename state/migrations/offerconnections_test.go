// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type OfferConnectionsExportSuite struct{}

var _ = gc.Suite(&OfferConnectionsExportSuite{})

func (s *OfferConnectionsExportSuite) TestExportOfferConnection(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationOfferConnection{
		s.migrationOfferConnection(ctrl, func(expect *MockMigrationOfferConnectionMockRecorder) {
			expect.OfferUUID().Return("offer-uuid")
			expect.RelationId().Return(1)
			expect.RelationKey().Return("relation-key")
			expect.SourceModelUUID().Return("source-model-uuid")
			expect.UserName().Return("fred")
		}),
	}

	offerConnection := NewMockOfferConnection(ctrl)

	source := NewMockOfferConnectionSource(ctrl)
	source.EXPECT().AllOfferConnections().Return(entities, nil)

	model := NewMockOfferConnectionModel(ctrl)
	model.EXPECT().AddOfferConnection(description.OfferConnectionArgs{
		OfferUUID:       "offer-uuid",
		RelationID:      1,
		RelationKey:     "relation-key",
		SourceModelUUID: "source-model-uuid",
		UserName:        "fred",
	}).Return(offerConnection)

	migration := ExportOfferConnections{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OfferConnectionsExportSuite) TestExportOfferConnectionFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationOfferConnection{
		NewMockMigrationOfferConnection(ctrl),
	}

	source := NewMockOfferConnectionSource(ctrl)
	source.EXPECT().AllOfferConnections().Return(entities, errors.New("fail"))

	model := NewMockOfferConnectionModel(ctrl)

	migration := ExportOfferConnections{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *OfferConnectionsExportSuite) migrationOfferConnection(ctrl *gomock.Controller, fn func(expect *MockMigrationOfferConnectionMockRecorder)) *MockMigrationOfferConnection {
	entity := NewMockMigrationOfferConnection(ctrl)
	fn(entity.EXPECT())
	return entity
}
