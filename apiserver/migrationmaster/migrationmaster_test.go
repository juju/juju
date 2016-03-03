// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/migrationmaster"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// Ensure that Backend remains compatible with *state.State
var _ migrationmaster.Backend = (*state.State)(nil)

type Suite struct {
	testing.BaseSuite

	backend    *fakeBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.backend = &fakeBackend{}
	migrationmaster.PatchState(s, s.backend)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
}

func (s *Suite) makeAPI() (*migrationmaster.API, error) {
	return migrationmaster.NewAPI(nil, s.resources, s.authorizer)
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationmaster.API {
	api, err := migrationmaster.NewAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) TestWatch(c *gc.C) {
	api := s.mustMakeAPI(c)

	watchResult, err := api.Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watchResult.NotifyWatcherId, gc.Not(gc.Equals), "")
}

func (s *Suite) TestWatchError(c *gc.C) {
	s.backend.watchError = errors.New("boom")
	api := s.mustMakeAPI(c)

	w, err := api.Watch()
	c.Assert(w, gc.Equals, params.NotifyWatchResult{})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestWatchNotEnvironManager(c *gc.C) {
	s.authorizer.EnvironManager = false

	api, err := s.makeAPI()
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

type fakeBackend struct {
	watchError error
}

func (b *fakeBackend) WatchForModelMigration() (state.NotifyWatcher, error) {
	if b.watchError != nil {
		return nil, b.watchError
	}
	return apiservertesting.NewFakeNotifyWatcher(), nil
}
