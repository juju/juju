// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/cleaner"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type CleanerSuite struct {
	coretesting.BaseSuite

	st         *mockState
	api        *cleaner.CleanerAPI
	authoriser apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&CleanerSuite{})

func (s *CleanerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.st = &mockState{&testing.Stub{}, false}
	cleaner.PatchState(s, s.st)
	var err error
	res := common.NewResources()
	s.api, err = cleaner.NewCleanerAPI(nil, res, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api, gc.NotNil)
}

func (s *CleanerSuite) TestNewCleanerAPIRequiresEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	api, err := cleaner.NewCleanerAPI(nil, nil, anAuthoriser)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(common.ServerError(err), jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *CleanerSuite) TestWatchCleanupsSuccess(c *gc.C) {
	_, err := s.api.WatchCleanups()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestWatchCleanupsFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom!"))
	s.st.watchCleanupsFails = true

	result, err := s.api.WatchCleanups()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Error(), gc.Equals, "boom!")
	s.st.CheckCallNames(c, "WatchCleanups")
}

func (s *CleanerSuite) TestCleanupSuccess(c *gc.C) {
	err := s.api.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "Cleanup")
}

func (s *CleanerSuite) TestCleanupFailure(c *gc.C) {
	s.st.SetErrors(errors.New("Boom!"))
	err := s.api.Cleanup()
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

func (st *mockState) Cleanup() error {
	st.MethodCall(st, "Cleanup")
	return st.NextErr()
}
