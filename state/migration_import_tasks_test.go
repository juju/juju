// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package state

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/description/v2"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"
)

type MigrationImportTasksSuite struct{}

var _ = gc.Suite(&MigrationImportTasksSuite{})

func (s *MigrationImportTasksSuite) TestImportApplicationOffers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	runner := ImportApplicationOfferRunner{
		OfferUUID: offerUUID.String(),
		model:     NewMockApplicationOfferInput(ctrl),
		runner:    NewMockTransactionRunner(ctrl),
	}

	entity := s.applicationOffer(ctrl)
	offerDoc := applicationOfferDoc{
		OfferUUID:              offerUUID.String(),
		OfferName:              "offer-name-foo",
		ApplicationName:        "foo",
		ApplicationDescription: "foo app description",
		Endpoints: map[string]string{
			"db": "db",
		},
	}

	runner.Add(runner.applicationOffers(entity))
	runner.Add(runner.applicationOfferDoc(offerDoc, entity))
	runner.Add(runner.docID)

	refOp := txn.Op{
		Assert: txn.DocMissing,
	}
	runner.Add(runner.applicationOffersRefOp(refOp))
	runner.Add(runner.transaction(offerDoc, refOp))

	err = runner.Run(ctrl)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportApplicationOffersTransactionFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	offerUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	runner := ImportApplicationOfferRunner{
		OfferUUID: offerUUID.String(),
		model:     NewMockApplicationOfferInput(ctrl),
		runner:    NewMockTransactionRunner(ctrl),
	}

	entity := s.applicationOffer(ctrl)
	offerDoc := applicationOfferDoc{
		OfferUUID:              offerUUID.String(),
		OfferName:              "offer-name-foo",
		ApplicationName:        "foo",
		ApplicationDescription: "foo app description",
		Endpoints: map[string]string{
			"db": "db",
		},
	}

	runner.Add(runner.applicationOffers(entity))
	runner.Add(runner.applicationOfferDoc(offerDoc, entity))
	runner.Add(runner.docID)

	refOp := txn.Op{
		Assert: txn.DocMissing,
	}
	runner.Add(runner.applicationOffersRefOp(refOp))
	runner.Add(runner.transactionWithError(errors.New("fail")))

	err = runner.Run(ctrl)
	c.Assert(err, gc.ErrorMatches, "fail")
}

type ImportApplicationOfferRunner struct {
	OfferUUID string
	model     *MockApplicationOfferInput
	runner    *MockTransactionRunner
	ops       []func(*gomock.Controller)
}

func (s *ImportApplicationOfferRunner) Add(fn func(*gomock.Controller)) {
	s.ops = append(s.ops, fn)
}

func (s *ImportApplicationOfferRunner) Run(ctrl *gomock.Controller) error {
	for _, v := range s.ops {
		v(ctrl)
	}

	m := ImportApplicationOffer{}
	return m.Execute(s.model, s.runner)
}

func (s *ImportApplicationOfferRunner) applicationOffers(entity description.ApplicationOffer) func(ctrl *gomock.Controller) {
	return func(ctrl *gomock.Controller) {
		entities := []description.ApplicationOffer{
			entity,
		}
		s.model.EXPECT().Offers().Return(entities)
	}
}

func (s *ImportApplicationOfferRunner) applicationOfferDoc(offerDoc applicationOfferDoc, entity description.ApplicationOffer) func(ctrl *gomock.Controller) {
	return func(ctrl *gomock.Controller) {
		s.model.EXPECT().MakeApplicationOfferDoc(entity).Return(offerDoc, nil)
	}
}

func (s *ImportApplicationOfferRunner) applicationOffersRefOp(op txn.Op) func(ctrl *gomock.Controller) {
	return func(ctrl *gomock.Controller) {
		s.model.EXPECT().MakeIncApplicationOffersRefOp("foo").Return(op, nil)
	}
}

func (s *ImportApplicationOfferRunner) docID(ctrl *gomock.Controller) {
	s.model.EXPECT().DocID("foo").Return("ao#foo")
}

func (s *MigrationImportTasksSuite) applicationOffer(ctrl *gomock.Controller) description.ApplicationOffer {
	return NewMockApplicationOffer(ctrl)
}

func (s *ImportApplicationOfferRunner) transaction(offerDoc applicationOfferDoc, ops ...txn.Op) func(ctrl *gomock.Controller) {
	return func(ctrl *gomock.Controller) {
		s.runner.EXPECT().RunTransaction(append([]txn.Op{
			{
				C:      applicationOffersC,
				Id:     "ao#foo",
				Assert: txn.DocMissing,
				Insert: offerDoc,
			},
		}, ops...)).Return(nil)
	}
}

func (s *ImportApplicationOfferRunner) transactionWithError(err error) func(ctrl *gomock.Controller) {
	return func(ctrl *gomock.Controller) {
		s.runner.EXPECT().RunTransaction(gomock.Any()).Return(err)
	}
}

func (s *MigrationImportTasksSuite) TestImportRemoteApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	status := s.status(ctrl)

	entity0 := s.remoteApplication(ctrl, status)
	entities := []description.RemoteApplication{
		entity0,
	}

	appDoc := &remoteApplicationDoc{
		Name:      "remote-application",
		URL:       "me/model.rainbow",
		OfferUUID: "offer-uuid",
		Endpoints: []remoteEndpointDoc{
			{Name: "db", Interface: "mysql"},
			{Name: "db-admin", Interface: "mysql-root"},
		},
		Spaces: []remoteSpaceDoc{
			{CloudType: "ec2"},
		},
	}
	statusDoc := statusDoc{}
	statusOp := txn.Op{}

	model := NewMockRemoteApplicationsInput(ctrl)
	model.EXPECT().RemoteApplications().Return(entities)
	model.EXPECT().MakeRemoteApplicationDoc(entity0).Return(appDoc)
	model.EXPECT().NewRemoteApplication(appDoc).Return(&RemoteApplication{
		doc: *appDoc,
	})
	model.EXPECT().MakeStatusDoc(status).Return(statusDoc)
	model.EXPECT().MakeStatusOp("c#remote-application", statusDoc).Return(statusOp)
	model.EXPECT().DocID("remote-application").Return("c#remote-application")

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      applicationsC,
			Id:     "remote-application",
			Assert: txn.DocMissing,
		},
		{
			C:      remoteApplicationsC,
			Id:     "c#remote-application",
			Assert: txn.DocMissing,
			Insert: appDoc,
		},
		statusOp,
	}).Return(nil)

	m := ImportRemoteApplications{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

// A Remote Application with a missing status field is a valid remote
// application and should be correctly imported.
func (s *MigrationImportTasksSuite) TestImportRemoteApplicationsWithMissingStatusField(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.remoteApplication(ctrl, nil)

	entities := []description.RemoteApplication{
		entity0,
	}

	appDoc := &remoteApplicationDoc{
		Name:      "remote-application",
		URL:       "me/model.rainbow",
		OfferUUID: "offer-uuid",
		Endpoints: []remoteEndpointDoc{
			{Name: "db", Interface: "mysql"},
			{Name: "db-admin", Interface: "mysql-root"},
		},
		Spaces: []remoteSpaceDoc{
			{CloudType: "ec2"},
		},
	}

	model := NewMockRemoteApplicationsInput(ctrl)
	model.EXPECT().RemoteApplications().Return(entities)
	model.EXPECT().MakeRemoteApplicationDoc(entity0).Return(appDoc)
	model.EXPECT().NewRemoteApplication(appDoc).Return(&RemoteApplication{
		doc: *appDoc,
	})
	model.EXPECT().DocID("remote-application").Return("c#remote-application")

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      applicationsC,
			Id:     "remote-application",
			Assert: txn.DocMissing,
		},
		{
			C:      remoteApplicationsC,
			Id:     "c#remote-application",
			Assert: txn.DocMissing,
			Insert: appDoc,
		},
	}).Return(nil)

	m := ImportRemoteApplications{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) remoteApplication(ctrl *gomock.Controller, status description.Status) description.RemoteApplication {
	entity := NewMockRemoteApplication(ctrl)
	entity.EXPECT().Status().Return(status)
	return entity
}

func (s *MigrationImportTasksSuite) status(ctrl *gomock.Controller) description.Status {
	entity := NewMockStatus(ctrl)
	return entity
}

func (s *MigrationImportTasksSuite) TestImportRemoteEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.remoteEntity(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc")
	entity1 := s.remoteEntity(ctrl, "ctrl-uuid-3", "aaa-bbb-zzz")

	entities := []description.RemoteEntity{
		entity0,
		entity1,
	}

	model := NewMockRemoteEntitiesInput(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)
	model.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")
	model.EXPECT().DocID("ctrl-uuid-3").Return("ctrl-uuid-3")

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

	m := ImportRemoteEntities{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportRemoteEntitiesWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.RemoteEntity{}

	model := NewMockRemoteEntitiesInput(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	// No call to RunTransaction if there are no operations.

	m := ImportRemoteEntities{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportRemoteEntitiesWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.remoteEntity(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc")

	entities := []description.RemoteEntity{
		entity0,
	}

	model := NewMockRemoteEntitiesInput(ctrl)
	model.EXPECT().RemoteEntities().Return(entities)
	model.EXPECT().DocID("ctrl-uuid-2").Return("uuid-2")

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

	m := ImportRemoteEntities{}
	err := m.Execute(model, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportTasksSuite) remoteEntity(ctrl *gomock.Controller, id, token string) *MockRemoteEntity {
	entity := NewMockRemoteEntity(ctrl)
	entity.EXPECT().ID().Return(id)
	entity.EXPECT().Token().Return(token)
	return entity
}

func (s *MigrationImportTasksSuite) TestImportRelationNetworks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.relationNetwork(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc", []string{"10.0.1.0/16"})
	entity1 := s.relationNetwork(ctrl, "ctrl-uuid-3", "aaa-bbb-zzz", []string{"10.0.0.1/24"})

	entities := []description.RelationNetwork{
		entity0,
		entity1,
	}

	model := NewMockRelationNetworksInput(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)
	model.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")
	model.EXPECT().DocID("ctrl-uuid-3").Return("ctrl-uuid-3")

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

	m := ImportRelationNetworks{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportRelationNetworksWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.RelationNetwork{}

	model := NewMockRelationNetworksInput(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	// No call to RunTransaction if there are no operations.

	m := ImportRelationNetworks{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportRelationNetworksWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.relationNetwork(ctrl, "ctrl-uuid-2", "xxx-yyy-ccc", []string{"10.0.1.0/16"})

	entities := []description.RelationNetwork{
		entity0,
	}

	model := NewMockRelationNetworksInput(ctrl)
	model.EXPECT().RelationNetworks().Return(entities)
	model.EXPECT().DocID("ctrl-uuid-2").Return("ctrl-uuid-2")

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

	m := ImportRelationNetworks{}
	err := m.Execute(model, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportTasksSuite) relationNetwork(ctrl *gomock.Controller, id, key string, cidrs []string) *MockRelationNetwork {
	entity := NewMockRelationNetwork(ctrl)
	entity.EXPECT().ID().Return(id)
	entity.EXPECT().RelationKey().Return(key)
	entity.EXPECT().CIDRS().Return(cidrs)
	return entity
}

func (s *MigrationImportTasksSuite) TestImportExternalControllers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.externalController(ctrl, "ctrl-uuid-2", "magic", "magic-cert", []string{"10.0.1.1"}, []string{"xxxx-yyyy-zzzz"})
	entity1 := s.externalController(ctrl, "ctrl-uuid-3", "foo", "foo-cert", []string{"10.0.2.24"}, []string{"aaaa-bbbb-cccc"})

	entities := []description.ExternalController{
		entity0,
		entity1,
	}

	doc0 := externalControllerDoc{
		Id:     "ctrl-uuid-2",
		Addrs:  []string{"10.0.1.1"},
		Alias:  "magic",
		CACert: "magic-cert",
		Models: []string{"xxxx-yyyy-zzzz"},
	}
	doc1 := externalControllerDoc{
		Id:     "ctrl-uuid-3",
		Addrs:  []string{"10.0.2.24"},
		Alias:  "foo",
		CACert: "foo-cert",
		Models: []string{"aaaa-bbbb-cccc"},
	}
	ops := []txn.Op{
		{
			C:      externalControllersC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: doc0,
		},
		{
			C:      externalControllersC,
			Id:     "ctrl-uuid-3",
			Assert: txn.DocMissing,
			Insert: doc1,
		},
	}

	model := NewMockExternalControllersInput(ctrl)
	model.EXPECT().ExternalControllers().Return(entities)
	gomock.InOrder(
		model.EXPECT().ExternalControllerDoc("ctrl-uuid-2").Return(nil, nil),
		model.EXPECT().MakeExternalControllerOp(doc0, nil).Return(ops[0]),
		model.EXPECT().ExternalControllerDoc("ctrl-uuid-3").Return(nil, nil),
		model.EXPECT().MakeExternalControllerOp(doc1, nil).Return(ops[1]),
	)

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction(ops).Return(nil)

	m := ImportExternalControllers{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportExternalControllersWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.ExternalController{}

	model := NewMockExternalControllersInput(ctrl)
	model.EXPECT().ExternalControllers().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	// No call to RunTransaction if there are no operations.

	m := ImportExternalControllers{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportExternalControllersWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.externalController(ctrl, "ctrl-uuid-2", "magic", "magic-cert", []string{"10.0.1.1"}, []string{"xxxx-yyyy-zzzz"})

	entities := []description.ExternalController{
		entity0,
	}

	doc0 := externalControllerDoc{
		Id:     "ctrl-uuid-2",
		Addrs:  []string{"10.0.1.1"},
		Alias:  "magic",
		CACert: "magic-cert",
		Models: []string{"xxxx-yyyy-zzzz"},
	}
	ops := []txn.Op{
		{
			C:      externalControllersC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: doc0,
		},
	}

	model := NewMockExternalControllersInput(ctrl)
	model.EXPECT().ExternalControllers().Return(entities)
	model.EXPECT().ExternalControllerDoc("ctrl-uuid-2").Return(nil, nil)
	model.EXPECT().MakeExternalControllerOp(doc0, nil).Return(ops[0])

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      externalControllersC,
			Id:     "ctrl-uuid-2",
			Assert: txn.DocMissing,
			Insert: externalControllerDoc{
				Id:     "ctrl-uuid-2",
				Addrs:  []string{"10.0.1.1"},
				Alias:  "magic",
				CACert: "magic-cert",
				Models: []string{"xxxx-yyyy-zzzz"},
			},
		},
	}).Return(errors.New("fail"))

	m := ImportExternalControllers{}
	err := m.Execute(model, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportTasksSuite) externalController(ctrl *gomock.Controller, id, alias, caCert string, addrs, models []string) *MockExternalController {
	entity := NewMockExternalController(ctrl)
	entity.EXPECT().ID().Return(names.NewControllerTag(id))
	entity.EXPECT().Alias().Return(alias)
	entity.EXPECT().CACert().Return(caCert)
	entity.EXPECT().Addrs().Return(addrs)
	entity.EXPECT().Models().Return(models)
	return entity
}

func (s *MigrationImportTasksSuite) TestImportFirewallRules(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.firewallRule(ctrl, "ssh", "ssh", []string{"192.168.0.1/24"})
	entity1 := s.firewallRule(ctrl, "ssh", "ssh", []string{"10.0.0.1/16"})

	entities := []description.FirewallRule{
		entity0,
		entity1,
	}

	model := NewMockFirewallRulesInput(ctrl)
	model.EXPECT().FirewallRules().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      firewallRulesC,
			Id:     "ssh",
			Assert: txn.DocMissing,
			Insert: firewallRulesDoc{
				Id:               "ssh",
				WellKnownService: "ssh",
				WhitelistCIDRS:   []string{"192.168.0.1/24"},
			},
		},
		{
			C:      firewallRulesC,
			Id:     "ssh",
			Assert: txn.DocMissing,
			Insert: firewallRulesDoc{
				Id:               "ssh",
				WellKnownService: "ssh",
				WhitelistCIDRS:   []string{"10.0.0.1/16"},
			},
		},
	}).Return(nil)

	m := ImportFirewallRules{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportFirewallRulesWithNoEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []description.FirewallRule{}

	model := NewMockFirewallRulesInput(ctrl)
	model.EXPECT().FirewallRules().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	// No call to RunTransaction if there are no operations.

	m := ImportFirewallRules{}
	err := m.Execute(model, runner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationImportTasksSuite) TestImportFirewallRulesWithTransactionRunnerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entity0 := s.firewallRule(ctrl, "ssh", "ssh", []string{"192.168.0.1/24"})

	entities := []description.FirewallRule{
		entity0,
	}

	model := NewMockFirewallRulesInput(ctrl)
	model.EXPECT().FirewallRules().Return(entities)

	runner := NewMockTransactionRunner(ctrl)
	runner.EXPECT().RunTransaction([]txn.Op{
		{
			C:      firewallRulesC,
			Id:     "ssh",
			Assert: txn.DocMissing,
			Insert: firewallRulesDoc{
				Id:               "ssh",
				WellKnownService: "ssh",
				WhitelistCIDRS:   []string{"192.168.0.1/24"},
			},
		},
	}).Return(errors.New("fail"))

	m := ImportFirewallRules{}
	err := m.Execute(model, runner)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *MigrationImportTasksSuite) firewallRule(ctrl *gomock.Controller, id, service string, whitelist []string) *MockFirewallRule {
	entity := NewMockFirewallRule(ctrl)
	entity.EXPECT().WellKnownService().Return(service)
	entity.EXPECT().WhitelistCIDRs().Return(whitelist)
	return entity
}
