// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/cleaner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/objectstore"
	applicationservice "github.com/juju/juju/domain/application/service"
	machineservice "github.com/juju/juju/domain/machine/service"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	unitservice "github.com/juju/juju/domain/unit/service"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type CleanerSuite struct {
	coretesting.BaseSuite

	st                 *mockState
	machineService     *machineservice.Service
	applicationService *applicationservice.Service
	unitService        *unitservice.Service
	api                *cleaner.CleanerAPI
	authoriser         apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&CleanerSuite{})

func (s *CleanerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	s.st = &mockState{&testing.Stub{}, false}
	cleaner.PatchState(s, s.st)
	var err error
	res := common.NewResources()
	s.machineService = machineservice.NewService(nil)
	s.applicationService = applicationservice.NewService(nil, storage.NotImplementedProviderRegistry{}, nil, loggo.GetLogger("test"))
	s.unitService = unitservice.NewService(nil)
	s.api, err = cleaner.NewCleanerAPI(facadetest.ModelContext{
		Resources_: res,
		Auth_:      s.authoriser,
		ServiceFactory_: servicefactorytesting.NewTestingServiceFactory().
			WithMachineService(func() *machineservice.Service {
				return s.machineService
			}).
			WithApplicationService(func() *applicationservice.Service {
				return s.applicationService
			}).
			WithUnitService(func() *unitservice.Service {
				return s.unitService
			}),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api, gc.NotNil)
}

func (s *CleanerSuite) TestNewCleanerAPIRequiresController(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Controller = false
	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Auth_: anAuthoriser,
	})
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(apiservererrors.ServerError(err), jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *CleanerSuite) TestWatchCleanupsSuccess(c *gc.C) {
	_, err := s.api.WatchCleanups(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestWatchCleanupsFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom!"))
	s.st.watchCleanupsFails = true

	result, err := s.api.WatchCleanups(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Error(), gc.Equals, "boom!")
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestCleanupSuccess(c *gc.C) {
	err := s.api.Cleanup(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "Cleanup")
	s.st.CheckCalls(c, []testing.StubCall{{
		FuncName: "Cleanup",
		Args:     []any{s.machineService, s.applicationService, s.unitService},
	}})
}

func (s *CleanerSuite) TestCleanupFailure(c *gc.C) {
	s.st.SetErrors(errors.New("Boom!"))
	err := s.api.Cleanup(context.Background())
	c.Assert(err, gc.ErrorMatches, "Boom!")
	s.st.CheckCallNames(c, "Cleanup")
}

type mockState struct {
	*testing.Stub
	watchCleanupsFails bool
}

type cleanupWatcher struct {
	out chan struct{}
	st  *mockState
}

func (w *cleanupWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *cleanupWatcher) Stop() error {
	return nil
}

func (w *cleanupWatcher) Kill() {
}

func (w *cleanupWatcher) Wait() error {
	return nil
}

func (w *cleanupWatcher) Err() error {
	return w.st.NextErr()
}

func (st *mockState) WatchCleanups() state.NotifyWatcher {
	w := &cleanupWatcher{
		out: make(chan struct{}, 1),
		st:  st,
	}
	if st.watchCleanupsFails {
		close(w.out)
	} else {
		w.out <- struct{}{}
	}
	st.MethodCall(st, "WatchCleanups")
	return w
}

func (st *mockState) Cleanup(_ context.Context, _ objectstore.ObjectStore, mr state.MachineRemover, ar state.ApplicationRemover, ur state.UnitRemover) error {
	st.MethodCall(st, "Cleanup", mr, ar, ur)
	return st.NextErr()
}
