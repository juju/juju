// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package state

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/description"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"
)

type MigrationImportSuite struct{}

var _ = gc.Suite(&MigrationImportSuite{})

func (s *MigrationImportSuite) TestImportRemoteEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.remoteEntity(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc")
	entity1 := s.remoteEntity(ctrl, "ctrl-uuid-3", "aaa-bbb-zzz")

	entities := []description.RemoteEntity{
		entity0,
		entity1,
	}

	model := NewMockModelRemoteEntities(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	docNamespace.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")
	docNamespace.EXPECT().DocID("ctrl-uuid-3").Return("ctrl-uuid-3")
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      remoteEntitiesC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				DocID: "ctrl-uuid-2",
				Token: "xxx-yyy-ccc",
			},
		},
		{
			C:      remoteEntitiesC,
			Id:     "ctrl-uuid-3",
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				DocID: "ctrl-uuid-3",
				Token: "aaa-bbb-zzz",
			},
		},
	}).Return(nil)

	err := importRemoteEntities(model, docNamespace, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportSuite) TestImportRemoteEntitiesWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.RemoteEntity{}

	model := NewMockModelRemoteEntities(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{})

	err := importRemoteEntities(model, docNamespace, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportSuite) TestImportRemoteEntitiesWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.remoteEntity(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc")

	entities := []description.RemoteEntity{
		entity0,
	}

	model := NewMockModelRemoteEntities(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	docNamespace.EXPECT().DocID("ctrl-uuid-2").Return("uuid-2")
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      remoteEntitiesC,
			Id:     "uuid-2",
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				DocID: "uuid-2",
				Token: "xxx-yyy-ccc",
			},
		},
	}).Return(errors.New("fail"))

	err := importRemoteEntities(model, docNamespace, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportSuite) remoteEntity(ctrl *gomock.Controller, id, token string) *MockRemoteEntity {
	entity := NewMockRemoteEntity(ctrl)
	entity.EXPECT().ID().Return(id)
	entity.EXPECT().Token().Return(token)
	return entity
}

func (s *MigrationImportSuite) TestImportRelationNetworks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.relationNetwork(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc", []string{"10.0.1.0/16"})
	entity1 := s.relationNetwork(ctrl, "ctrl-uuid-3", "aaa-bbb-zzz", []string{"10.0.0.1/24"})

	entities := []description.RelationNetwork{
		entity0,
		entity1,
	}

	model := NewMockModelRelationNetworks(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	docNamespace.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")
	docNamespace.EXPECT().DocID("ctrl-uuid-3").Return("ctrl-uuid-3")
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      relationNetworksC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: relationNetworksDoc{
				Id:          "ctrl-uuid-2",
				RelationKey: "xxx-yyy-ccc",
				CIDRS:       []string{"10.0.1.0/16"},
			},
		},
		{
			C:      relationNetworksC,
			Id:     "ctrl-uuid-3",
			Assert: txn.DocMissing,
			Insert: relationNetworksDoc{
				Id:          "ctrl-uuid-3",
				RelationKey: "aaa-bbb-zzz",
				CIDRS:       []string{"10.0.0.1/24"},
			},
		},
	}).Return(nil)

	err := importRelationNetworks(model, docNamespace, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportSuite) TestImportRelationNetworksWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.RelationNetwork{}

	model := NewMockModelRelationNetworks(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{}).Return(nil)

	err := importRelationNetworks(model, docNamespace, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportSuite) TestImportRelationNetworksWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.relationNetwork(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc", []string{"10.0.1.0/16"})

	entities := []description.RelationNetwork{
		entity0,
	}

	model := NewMockModelRelationNetworks(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)
	docNamespace := NewMockDocModelNamespace(ctrl)
	docNamespace.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")
	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      relationNetworksC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: relationNetworksDoc{
				Id:          "ctrl-uuid-2",
				RelationKey: "xxx-yyy-ccc",
				CIDRS:       []string{"10.0.1.0/16"},
			},
		},
	}).Return(errors.New("fail"))

	err := importRelationNetworks(model, docNamespace, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportSuite) relationNetwork(ctrl *gomock.Controller, id, key string, cidrs []string) *MockRelationNetwork {
	entity := NewMockRelationNetwork(ctrl)
	entity.EXPECT().ID().Return(id)
	entity.EXPECT().RelationKey().Return(key)
	entity.EXPECT().CIDRS().Return(cidrs)
	return entity
}
