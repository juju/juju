// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/migrationminion"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type Suite struct {
	coretesting.BaseSuite

	authorizer            apiservertesting.FakeAuthorizer
	modelMigrationService *MockModelMigrationService
	watcherRegistry       *facademocks.MockWatcherRegistry
}

func TestSuite(t *testing.T) {
	tc.Run(t, &Suite{})
}

func (s *Suite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}
}

func (s *Suite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelMigrationService = NewMockModelMigrationService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	c.Cleanup(func() {
		s.modelMigrationService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *Suite) TestAuthMachineAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.authorizer.Tag = names.NewMachineTag("42")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthUnitAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.authorizer.Tag = names.NewUnitTag("foo/0")
	s.mustMakeAPI(c)
}

func (s *Suite) TestAuthNotAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.authorizer.Tag = names.NewUserTag("dorothy")
	_, err := s.makeAPI()
	c.Assert(err, tc.Equals, apiservererrors.ErrPerm)
}

func (s *Suite) TestWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.CleanKill(c, w)

	s.modelMigrationService.EXPECT().WatchForMigration(gomock.Any()).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("123", nil)
	api := s.mustMakeAPI(c)
	result, err := api.Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.NotifyWatcherId, tc.Equals, "123")
}

func (s *Suite) TestReportMachine(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.modelMigrationService.EXPECT().ReportFromMachine(gomock.Any(), machine.Name("99"), migration.IMPORT).Return(nil)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("99"),
	}
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "IMPORT",
		Success:     true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *Suite) TestReportUnit(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.modelMigrationService.EXPECT().ReportFromUnit(gomock.Any(), unit.Name("a/123"), migration.IMPORT).Return(nil)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUnitTag("a/123"),
	}
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "IMPORT",
		Success:     true,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *Suite) TestReportInvalidPhase(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "WTF",
		Success:     true,
	})
	c.Assert(err, tc.ErrorMatches, "unable to parse phase")
}

func (s *Suite) TestReportNoSuchMigration(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.modelMigrationService.EXPECT().ReportFromMachine(gomock.Any(), machine.Name("99"), migration.QUIESCE).Return(errors.NotFoundf("model"))
	api := s.mustMakeAPI(c)
	err := api.Report(c.Context(), params.MinionReport{
		MigrationId: "id",
		Phase:       "QUIESCE",
		Success:     false,
	})
	c.Assert(errors.Cause(err), tc.ErrorMatches, `model not found`)
}

func (s *Suite) makeAPI() (*migrationminion.API, error) {
	return migrationminion.NewAPI(s.watcherRegistry, s.authorizer, s.modelMigrationService)
}

func (s *Suite) mustMakeAPI(c *tc.C) *migrationminion.API {
	api, err := s.makeAPI()
	c.Assert(err, tc.ErrorIsNil)
	return api
}
