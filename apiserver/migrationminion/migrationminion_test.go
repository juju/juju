// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/migrationminion"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// Ensure that Backend remains compatible with *state.State
var _ migrationminion.Backend = (*state.State)(nil)

type Suite struct {
	testing.BaseSuite

	backend    *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.backend = &stubBackend{}
	migrationminion.PatchState(s, s.backend)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}
}

func (s *Suite) TestAuthMachineAgent(c *gc.C) {
	s.authorizer.Tag = names.NewMachineTag("42")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthUnitAgent(c *gc.C) {
	s.authorizer.Tag = names.NewUnitTag("foo/0")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthNotAgent(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("dorothy")
	_, err := s.makeAPI()
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *Suite) TestWatchError(c *gc.C) {
	s.backend.watchError = errors.New("boom")
	api := s.mustMakeAPI(c)
	_, err := api.Watch()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

func (s *Suite) TestWatch(c *gc.C) {
	api := s.mustMakeAPI(c)
	result, err := api.Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.resources.Get(result.NotifyWatcherId), gc.NotNil)
}

func (s *Suite) makeAPI() (*migrationminion.API, error) {
	return migrationminion.NewAPI(nil, s.resources, s.authorizer)
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationminion.API {
	api, err := migrationminion.NewAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

type stubBackend struct {
	migrationminion.Backend
	watchError error
}

func (b *stubBackend) WatchMigrationStatus() (state.NotifyWatcher, error) {
	if b.watchError != nil {
		return nil, b.watchError
	}
	return apiservertesting.NewFakeNotifyWatcher(), nil
}
