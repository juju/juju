// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/feature"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

//go:generate go run go.uber.org/mock/mockgen -package uniter_test -destination package_mocks_test.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackend,LXDProfileMachine,LXDProfileUnit,LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2,LXDProfileModelV2,SpaceService

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// uniterSuiteBase implements common testing suite for all API versions.
// It is not intended to be used directly or registered as a suite,
// but embedded.
type uniterSuiteBase struct {
	testing.ApiServerSuite

	authorizer        apiservertesting.FakeAuthorizer
	resources         *common.Resources
	leadershipRevoker *leadershipRevoker
	uniter            *uniter.UniterAPI

	machine0          *state.Machine
	machine1          *state.Machine
	wpCharm           *state.Charm
	wordpress         *state.Application
	wordpressUnit     *state.Unit
	mysqlCharm        *state.Charm
	mysql             *state.Application
	mysqlUnit         *state.Unit
	leadershipChecker *fakeLeadershipChecker

	store objectstore.ObjectStore
}

func (s *uniterSuiteBase) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.Features: feature.RawK8sSpec,
	}
	s.WithLeaseManager = true

	s.ApiServerSuite.SetUpTest(c)
	s.ApiServerSuite.SeedCAASCloud(c)

	s.setupState(c)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming the wordpress unit has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.wordpressUnit.Tag(),
	}
	s.leadershipRevoker = &leadershipRevoker{
		revoked: set.NewStrings(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.leadershipChecker = &fakeLeadershipChecker{false}
	s.uniter = s.newUniterAPI(c, s.ControllerModel(c).State(), s.authorizer)
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	s.store = testing.NewObjectStore(c, s.ControllerModelUUID())
}

// setupState creates 2 machines, 2 services and adds a unit to each service.
func (s *uniterSuiteBase) setupState(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	s.machine0 = f.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.machine1 = f.MakeMachine(c, &factory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})

	s.wpCharm = f.MakeCharm(c, &factory.CharmParams{
		Name:     "wordpress",
		Revision: "3",
	})
	s.wordpress = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.wpCharm,
	}, nil)
	s.wordpressUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	s.mysqlCharm = f.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: s.mysqlCharm,
	}, nil)
	s.mysqlUnit = f.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
}

func (s *uniterSuiteBase) facadeContext(c *gc.C) facadetest.ModelContext {
	return facadetest.ModelContext{
		State_:             s.ControllerModel(c).State(),
		StatePool_:         s.StatePool(),
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.leadershipChecker,
		ServiceFactory_:    s.DefaultModelServiceFactory(c),
		ObjectStore_:       testing.NewObjectStore(c, s.ControllerModelUUID()),
	}
}

func (s *uniterSuiteBase) newUniterAPI(c *gc.C, st *state.State, auth facade.Authorizer) *uniter.UniterAPI {
	facadeContext := s.facadeContext(c)
	facadeContext.State_ = st
	facadeContext.Auth_ = auth
	facadeContext.LeadershipRevoker_ = s.leadershipRevoker
	uniterAPI, err := uniter.NewUniterAPI(context.Background(), facadeContext)
	c.Assert(err, jc.ErrorIsNil)
	return uniterAPI
}

func (s *uniterSuiteBase) addRelation(c *gc.C, first, second string) *state.Relation {
	st := s.ControllerModel(c).State()
	eps, err := st.InferEndpoints(first, second)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *uniterSuiteBase) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

// TODO (manadart 2020-12-07): This should form the basis of a SetUpTest method
// in a new suite.
// If we are testing a CAAS model, it is a waste of resources to do preamble
// for an IAAS model.
func (s *uniterSuiteBase) setupCAASModel(c *gc.C) (*apiuniter.Client, *state.CAASModel, *state.Application, *state.Unit) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := f.MakeCAASModel(c, nil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.CleanupSuite.AddCleanup(func(*gc.C) { _ = st.Close() })
	cm, err := m.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	f2, release := s.NewFactory(c, m.UUID())
	defer release()

	app := f2.MakeApplication(
		c, &factory.ApplicationParams{
			Name:  "gitlab",
			Charm: f2.MakeCharm(c, &factory.CharmParams{Name: "gitlab-k8s", Series: "focal"}),
		}, nil)
	unit := f2.MakeUnit(c, &factory.UnitParams{
		Application: app,
		SetCharmURL: true,
	})
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: unit.Tag(),
	}

	password, err := password.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	api := s.OpenModelAPIAs(c, st.ModelUUID(), unit.Tag(), password, "nonce")
	u, err := apiuniter.NewFromConnection(api)
	c.Assert(err, jc.ErrorIsNil)
	return u, cm, app, unit
}

type fakeBroker struct {
	caas.Broker
}

func (*fakeBroker) APIVersion() (string, error) {
	return "6.66", nil
}

type fakeToken struct {
	err error
}

func (t *fakeToken) Check() error {
	return t.err
}

type fakeLeadershipChecker struct {
	isLeader bool
}

type token struct {
	isLeader          bool
	unit, application string
}

func (t *token) Check() error {
	if !t.isLeader {
		return leadership.NewNotLeaderError(t.unit, t.application)
	}
	return nil
}

func (f *fakeLeadershipChecker) LeadershipCheck(applicationName, unitName string) leadership.Token {
	return &token{isLeader: f.isLeader, unit: unitName, application: applicationName}
}

func ptr[T any](v T) *T {
	return &v
}

type leadershipRevoker struct {
	revoked set.Strings
}

func (s *leadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
	return nil
}
