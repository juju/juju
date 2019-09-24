// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package state

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/description"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type MigrationExportSuite struct{}

var _ = gc.Suite(&MigrationExportSuite{})

func (s *MigrationExportSuite) TestExportRemoteEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []RemoteEntity{
		{docID: "controller-uuid-3", token: "aaa-bbb"},
		{docID: "controller-uuid-4", token: "ccc-yyy", macaroon: "macaroon-5"},
	}
	model := NewMockRemoteEntitiesModel(ctrl)
	model.EXPECT().AddRemoteEntity(description.RemoteEntityArgs{
		ID:    "controller-uuid-3",
		Token: "aaa-bbb",
	})
	model.EXPECT().AddRemoteEntity(description.RemoteEntityArgs{
		ID:       "controller-uuid-4",
		Token:    "ccc-yyy",
		Macaroon: "macaroon-5",
	})

	err := exportRemoteEntities(entities, model)
	c.Assert(err, jc.ErrorIsNil)
}
