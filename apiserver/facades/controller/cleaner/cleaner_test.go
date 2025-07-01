// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/cleaner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type CleanerSuite struct {
	coretesting.BaseSuite

	st         *mockState
	authoriser apiservertesting.FakeAuthorizer

	domainServices *MockDomainServices
}

func TestCleanerSuite(t *testing.T) {
	tc.Run(t, &CleanerSuite{})
}

func (s *CleanerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.domainServices = NewMockDomainServices(ctrl)
	s.domainServices.EXPECT().Machine()
	return ctrl
}

func (s *CleanerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	s.st = &mockState{&testhelpers.Stub{}, false}
	cleaner.PatchState(s, s.st)
}

func (s *CleanerSuite) TestNewCleanerAPIRequiresController(c *tc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Controller = false
	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Auth_: anAuthoriser,
	})
	c.Assert(api, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(apiservererrors.ServerError(err), tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *CleanerSuite) TestWatchCleanupsSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Resources_:      common.NewResources(),
		Auth_:           s.authoriser,
		DomainServices_: s.domainServices,
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = api.WatchCleanups(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestWatchCleanupsFailure(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Resources_:      common.NewResources(),
		Auth_:           s.authoriser,
		DomainServices_: s.domainServices,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.st.SetErrors(errors.New("boom!"))
	s.st.watchCleanupsFails = true

	result, err := api.WatchCleanups(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error.Error(), tc.Equals, "boom!")
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestCleanupSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Resources_:      common.NewResources(),
		Auth_:           s.authoriser,
		DomainServices_: s.domainServices,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = api.Cleanup(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	s.st.CheckCallNames(c, "Cleanup")
}

func (s *CleanerSuite) TestCleanupFailure(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	api, err := cleaner.NewCleanerAPI(facadetest.ModelContext{
		Resources_:      common.NewResources(),
		Auth_:           s.authoriser,
		DomainServices_: s.domainServices,
	})
	c.Assert(err, tc.ErrorIsNil)

	s.st.SetErrors(errors.New("Boom!"))
	err = api.Cleanup(c.Context())
	c.Assert(err, tc.ErrorMatches, "Boom!")
	s.st.CheckCallNames(c, "Cleanup")
}

type mockState struct {
	*testhelpers.Stub
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

func (st *mockState) Cleanup(_ context.Context, _ objectstore.ObjectStore, mr state.MachineRemover) error {
	st.MethodCall(st, "Cleanup", mr)
	return st.NextErr()
}
