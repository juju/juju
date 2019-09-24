// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package state

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/description"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
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

	source := NewMockRemoteEntitiesSource(ctrl)
	source.EXPECT().AllRemoteEntities().Return(entities, nil)

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

	err := exportRemoteEntities(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationExportSuite) TestExportRemoteEntitiesFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockRemoteEntitiesSource(ctrl)
	source.EXPECT().AllRemoteEntities().Return(nil, errors.New("fail"))

	model := NewMockRemoteEntitiesModel(ctrl)

	err := exportRemoteEntities(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationExportSuite) TestExportRelationNetworks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.relationNetworks(ctrl, "uuid-4", "wordpress:db mysql:server", []string{"192.168.1.0/16"})

	entities := []RelationNetworks{
		entity0,
	}

	source := NewMockRelationNetworksSource(ctrl)
	source.EXPECT().AllRelationNetworks().Return(entities, nil)

	model := NewMockRelationNetworksModel(ctrl)
	model.EXPECT().AddRelationNetwork(description.RelationNetworkArgs{
		ID:          names.NewControllerTag("uuid-4").String(),
		RelationKey: "wordpress:db mysql:server",
		CIDRS:       []string{"192.168.1.0/16"},
	})

	err := exportRelationNetworks(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationExportSuite) TestExportRelationNetworksFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockRelationNetworksSource(ctrl)
	source.EXPECT().AllRelationNetworks().Return(nil, errors.New("fail"))

	model := NewMockRelationNetworksModel(ctrl)

	err := exportRelationNetworks(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationExportSuite) relationNetworks(ctrl *gomock.Controller, id, relationKey string, cidrs []string) *MockRelationNetworks {
	entity := NewMockRelationNetworks(ctrl)
	entity.EXPECT().Id().Return(names.NewControllerTag(id).String())
	entity.EXPECT().RelationKey().Return(relationKey)
	entity.EXPECT().CIDRS().Return(cidrs)
	return entity
}
