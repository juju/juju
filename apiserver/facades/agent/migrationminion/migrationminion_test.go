// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Ensure that Backend remains compatible with *state.State
var _ migrationminion.Backend = (*state.State)(nil)

type Suite struct {
	coretesting.BaseSuite

	stub       *testhelpers.Stub
	backend    *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

func TestSuite(t *stdtesting.T) { tc.Run(t, &Suite{}) }
func (s *Suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.backend = &stubBackend{stub: s.stub}

	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}
}

func (s *Suite) TestAuthMachineAgent(c *tc.C) {
	s.authorizer.Tag = names.NewMachineTag("42")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthUnitAgent(c *tc.C) {
	s.authorizer.Tag = names.NewUnitTag("foo/0")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthApplicationAgent(c *tc.C) {
	s.authorizer.Tag = names.NewApplicationTag("foo")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthNotAgent(c *tc.C) {
	s.authorizer.Tag = names.NewUserTag("dorothy")
	_, err := s.makeAPI()
	c.Assert(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestWatch(c *tc.C) {
	api := s.mustMakeAPI(c)
	result, err := api.Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.resources.Get(result.NotifyWatcherId), tc.NotNil)
}

func (s *Suite) TestReport(c *tc.C) {
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "IMPORT",
		Success:     true,
	})
	c.Assert(err, tc.ErrorIsNil)
	s.stub.CheckCalls(c, []testhelpers.StubCall{
		{"Migration", []interface{}{"id"}},
		{"Report", []interface{}{s.authorizer.Tag, migration.IMPORT, true}},
	})
}

func (s *Suite) TestReportInvalidPhase(c *tc.C) {
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "WTF",
		Success:     true,
	})
	c.Assert(err, tc.ErrorMatches, "unable to parse phase")
}

func (s *Suite) TestReportNoSuchMigration(c *tc.C) {
	failure := errors.NotFoundf("model")
	s.backend.modelLookupErr = failure
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "QUIESCE",
		Success:     false,
	})
	c.Assert(errors.Cause(err), tc.Equals, failure)
}

func (s *Suite) makeAPI() (*migrationminion.API, error) {
	return migrationminion.NewAPI(s.backend, s.resources, s.authorizer)
}

func (s *Suite) mustMakeAPI(c *tc.C) *migrationminion.API {
	api, err := s.makeAPI()
	c.Assert(err, tc.ErrorIsNil)
	return api
}

type stubBackend struct {
	migrationminion.Backend
	stub           *testhelpers.Stub
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
	stub *testhelpers.Stub
}

func (m *stubModelMigration) SubmitMinionReport(tag names.Tag, phase migration.Phase, success bool) error {
	m.stub.AddCall("Report", tag, phase, success)
	return nil
}
