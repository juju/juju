// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type RemoteEntitiesExportSuite struct{}

var _ = gc.Suite(&RemoteEntitiesExportSuite{})

func (s *RemoteEntitiesExportSuite) TestExportRemoteEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteEntity{
		s.remoteEntity(ctrl, "uuid3", "aaa-bbb", ""),
		s.remoteEntity(ctrl, "uuid4", "ccc-yyy", "macaroon-5"),
	}

	source := NewMockRemoteEntitiesSource(ctrl)
	source.EXPECT().AllRemoteEntities().Return(entities, nil)

	model := NewMockRemoteEntitiesModel(ctrl)
	model.EXPECT().AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "controller-uuid3",
		Token: "aaa-bbb",
	})
	model.EXPECT().AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "controller-uuid4",
		Token: "ccc-yyy",
		// Note no macaroon.
	})

	migration := ExportRemoteEntities{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoteEntitiesExportSuite) TestExportRemoteEntitiesFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockRemoteEntitiesSource(ctrl)
	source.EXPECT().AllRemoteEntities().Return(nil, errors.New("fail"))

	model := NewMockRemoteEntitiesModel(ctrl)

	migration := ExportRemoteEntities{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *RemoteEntitiesExportSuite) remoteEntity(
	ctrl *gomock.Controller, id, token, macaroon string,
) *MockMigrationRemoteEntity {
	entity := NewMockMigrationRemoteEntity(ctrl)
	entity.EXPECT().ID().Return(names.NewControllerTag(id).String())
	entity.EXPECT().Token().Return(token)
	return entity
}
