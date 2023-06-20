// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// Ensure that Backend remains compatible with *state.State
var _ migrationminion.Backend = (*state.State)(nil)

type Suite struct {
	coretesting.BaseSuite

	stub            *testing.Stub
	backend         *stubBackend
	watcherRegistry facade.WatcherRegistry
	authorizer      apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.backend = &stubBackend{stub: s.stub}

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

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

func (s *Suite) TestAuthApplicationAgent(c *gc.C) {
	s.authorizer.Tag = names.NewApplicationTag("foo")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthNotAgent(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("dorothy")
	_, err := s.makeAPI()
	c.Assert(err, gc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestWatch(c *gc.C) {
	api := s.mustMakeAPI(c)
	result, err := api.Watch()
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.watcherRegistry.Get(result.NotifyWatcherId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *Suite) TestReport(c *gc.C) {
	api := s.mustMakeAPI(c)
	err := api.Report(params.MinionReport{
		MigrationId: "id",
		Phase:       "IMPORT",
		Success:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{
		{FuncName: "Migration", Args: []interface{}{"id"}},
		{FuncName: "Report", Args: []interface{}{s.authorizer.Tag, migration.IMPORT, true}},
	})
}

func (s *Suite) TestReportInvalidPhase(c *gc.C) {
	api := s.mustMakeAPI(c)
	err := api.Report(params.MinionReport{
		MigrationId: "id",
		Phase:       "WTF",
		Success:     true,
	})
	c.Assert(err, gc.ErrorMatches, "unable to parse phase")
}

func (s *Suite) TestReportNoSuchMigration(c *gc.C) {
	failure := errors.NotFoundf("model")
	s.backend.modelLookupErr = failure
	api := s.mustMakeAPI(c)
	err := api.Report(params.MinionReport{
		MigrationId: "id",
		Phase:       "QUIESCE",
		Success:     false,
	})
	c.Assert(errors.Cause(err), gc.Equals, failure)
}

func (s *Suite) makeAPI() (*migrationminion.API, error) {
	return migrationminion.NewAPI(s.backend, s.watcherRegistry, s.authorizer)
}

func (s *Suite) mustMakeAPI(c *gc.C) *migrationminion.API {
	api, err := s.makeAPI()
	c.Assert(err, jc.ErrorIsNil)
	return api
}

type stubBackend struct {
	migrationminion.Backend
	stub           *testing.Stub
	modelLookupErr error
}

func (b *stubBackend) WatchMigrationStatus() state.NotifyWatcher {
	b.stub.AddCall("WatchMigrationStatus")
	return apiservertesting.NewFakeNotifyWatcher()
}

func (b *stubBackend) Migration(id string) (state.ModelMigration, error) {
	b.stub.AddCall("Migration", id)
	if b.modelLookupErr != nil {
		return nil, b.modelLookupErr
	}
	return &stubModelMigration{stub: b.stub}, nil
}

type stubModelMigration struct {
	state.ModelMigration
	stub *testing.Stub
}

func (m *stubModelMigration) SubmitMinionReport(tag names.Tag, phase migration.Phase, success bool) error {
	m.stub.AddCall("Report", tag, phase, success)
	return nil
}
