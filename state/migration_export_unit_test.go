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

func (s *MigrationExportSuite) TestExportRemoteApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []*RemoteApplication{
		{
			doc: remoteApplicationDoc{
				Name:            "app-uuid-1",
				OfferUUID:       "offer-uuid",
				URL:             "me/model.foo",
				SourceModelUUID: "model-uuid-2",
				IsConsumerProxy: false,
				Bindings: map[string]string{
					"binding-key": "binding-value",
				},
				Endpoints: []remoteEndpointDoc{{
					Name: "app-uuid-1-endpoint-1",
				}},
				Spaces: []remoteSpaceDoc{{
					Name: "app-uuid-1-spaces-1",
					Subnets: []remoteSubnetDoc{{
						CIDR: "10.0.0.1/24",
					}},
				}},
			},
		},
	}

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)

	statusSource := NewMockStatusSource(ctrl)
	statusSource.EXPECT().StatusArgs("c#app-uuid-1").Return(description.StatusArgs{
		Value: "status-value",
	}, nil)

	remoteSpace := NewMockRemoteSpace(ctrl)
	remoteSpace.EXPECT().AddSubnet(description.SubnetArgs{
		CIDR: "10.0.0.1/24",
	})

	remoteApplication := NewMockRemoteApplication(ctrl)
	remoteApplication.EXPECT().SetStatus(description.StatusArgs{
		Value: "status-value",
	})
	remoteApplication.EXPECT().AddEndpoint(description.RemoteEndpointArgs{
		Name: "app-uuid-1-endpoint-1",
	})
	remoteApplication.EXPECT().AddSpace(description.RemoteSpaceArgs{
		Name: "app-uuid-1-spaces-1",
	}).Return(remoteSpace)

	model := NewMockRemoteApplicationModel(ctrl)
	model.EXPECT().AddRemoteApplication(description.RemoteApplicationArgs{
		Tag:             names.NewApplicationTag("app-uuid-1"),
		OfferUUID:       "offer-uuid",
		URL:             "me/model.foo",
		SourceModel:     names.NewModelTag("model-uuid-2"),
		IsConsumerProxy: false,
		Bindings: map[string]string{
			"binding-key": "binding-value",
		},
	}).Return(remoteApplication)

	err := exportRemoteApplications(source, statusSource, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationExportSuite) TestExportRemoteApplicationWithStatusArgsFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []*RemoteApplication{
		{
			doc: remoteApplicationDoc{
				Name:            "app-uuid-1",
				OfferUUID:       "offer-uuid",
				URL:             "me/model.foo",
				SourceModelUUID: "model-uuid-2",
				IsConsumerProxy: false,
				Bindings: map[string]string{
					"binding-key": "binding-value",
				},
			},
		},
	}

	source := NewMockRemoteApplicationSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)

	statusSource := NewMockStatusSource(ctrl)
	statusSource.EXPECT().StatusArgs("c#app-uuid-1").Return(description.StatusArgs{
		Value: "status-value",
	}, errors.New("fail"))

	remoteApplication := NewMockRemoteApplication(ctrl)

	model := NewMockRemoteApplicationModel(ctrl)
	model.EXPECT().AddRemoteApplication(description.RemoteApplicationArgs{
		Tag:             names.NewApplicationTag("app-uuid-1"),
		OfferUUID:       "offer-uuid",
		URL:             "me/model.foo",
		SourceModel:     names.NewModelTag("model-uuid-2"),
		IsConsumerProxy: false,
		Bindings: map[string]string{
			"binding-key": "binding-value",
		},
	}).Return(remoteApplication)

	err := exportRemoteApplications(source, statusSource, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

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
