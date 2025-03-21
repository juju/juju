// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/featureflag"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination clock_mock_test.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination package_mocks_test.go github.com/juju/juju/apiserver/facades/agent/uniter LXDProfileBackend,LXDProfileMachine,LXDProfileUnit,LXDProfileBackendV2,LXDProfileMachineV2,LXDProfileUnitV2,LXDProfileCharmV2
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination secret_mocks_test.go github.com/juju/juju/apiserver/facades/agent/uniter SecretService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination leadership_mocks_test.go github.com/juju/juju/core/leadership Checker,Token
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination legacy_service_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter ModelConfigService,ModelInfoService,NetworkService,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/uniter ApplicationService,StatusService,RelationService,ModelInfoService
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination watcher_registry_mock_test.go github.com/juju/juju/apiserver/facade WatcherRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package uniter -destination relation_mock_test.go github.com/juju/juju/domain/relation RelationUnitsWatcher

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

// uniterSuiteBase implements common testing suite for all API versions.
// It is not intended to be used directly or registered as a suite,
// but embedded.
//
// Suites embedding this base are skipped.
// Testing factory functionality is removed.
// Deprecated: Retained for test documentation purposes.
type uniterSuiteBase struct {
	testing.ApiServerSuite

	authorizer        apiservertesting.FakeAuthorizer
	resources         *common.Resources
	watcherRegistry   *MockWatcherRegistry
	leadershipRevoker *leadershipRevoker
	uniter            *uniter.UniterAPI

	leadershipChecker *fakeLeadershipChecker

	store objectstore.ObjectStore
}

func (s *uniterSuiteBase) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	return ctrl
}

func (s *uniterSuiteBase) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.Features: featureflag.RawK8sSpec,
	}
	s.WithLeaseManager = true

	s.ApiServerSuite.SetUpTest(c)
	s.ApiServerSuite.SeedCAASCloud(c)

	s.setupState(c)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming the wordpress unit has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("wordpress/0"),
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
func (s *uniterSuiteBase) setupState(c *gc.C) {}

func (s *uniterSuiteBase) facadeContext(c *gc.C) facadetest.ModelContext {
	return facadetest.ModelContext{
		State_:             s.ControllerModel(c).State(),
		StatePool_:         s.StatePool(),
		Resources_:         s.resources,
		WatcherRegistry_:   s.watcherRegistry,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.leadershipChecker,
		DomainServices_:    s.DefaultModelDomainServices(c),
		ObjectStore_:       testing.NewObjectStore(c, s.ControllerModelUUID()),
		Logger_:            loggertesting.WrapCheckLog(c),
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

func (s *uniterSuiteBase) newUniterAPIv19(c *gc.C, st *state.State, auth facade.Authorizer) *uniter.UniterAPIv19 {
	facadeContext := s.facadeContext(c)
	facadeContext.State_ = st
	facadeContext.Auth_ = auth
	facadeContext.LeadershipRevoker_ = s.leadershipRevoker
	uniterAPI, err := uniter.NewUniterAPIv19(context.Background(), facadeContext)
	c.Assert(err, jc.ErrorIsNil)
	return uniterAPI
}

// TODO (manadart 2020-12-07): This should form the basis of a SetUpTest method
// in a new suite.
// If we are testing a CAAS model, it is a waste of resources to do preamble
// for an IAAS model.
func (s *uniterSuiteBase) setupCAASModel(c *gc.C) (*apiuniter.Client, *state.CAASModel, *state.Application, *state.Unit) {
	return nil, nil, nil, nil
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

type leadershipRevoker struct {
	revoked set.Strings
}

func (s *leadershipRevoker) RevokeLeadership(applicationName string, unitName unit.Name) error {
	s.revoked.Add(unitName.String())
	return nil
}
