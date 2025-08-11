// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/migrationflag"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migrationflag_test -destination mocks_test.go github.com/juju/juju/apiserver/facades/agent/migrationflag ModelMigrationService

type FacadeSuite struct {
	testhelpers.IsolationSuite

	modelMigrationService *MockModelMigrationService
	watcherRegistry       *facademocks.MockWatcherRegistry
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &FacadeSuite{})
}

func (s *FacadeSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelMigrationService = NewMockModelMigrationService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	c.Cleanup(func() {
		s.modelMigrationService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *FacadeSuite) modelAuthorisation(c *tc.C) common.GetAuthFunc {
	return func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			c.Check(tag, tc.FitsTypeOf, names.ModelTag{})
			return tag.Id() == coretesting.ModelTag.Id()
		}, nil
	}
}

func (s *FacadeSuite) TestAcceptsMachineAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	facade, err := migrationflag.New(
		s.watcherRegistry,
		agentAuth{machine: true},
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (s *FacadeSuite) TestAcceptsUnitAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	facade, err := migrationflag.New(
		s.watcherRegistry,
		agentAuth{machine: true},
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (s *FacadeSuite) TestRejectsNonAgent(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	facade, err := migrationflag.New(
		s.watcherRegistry,
		agentAuth{},
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, tc.IsNil)
}

func (s *FacadeSuite) TestPhaseSuccess(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	mig := modelmigration.Migration{
		Phase: migration.REAP,
	}
	s.modelMigrationService.EXPECT().Migration(gomock.Any()).Return(mig, nil).Times(2)

	facade, err := migrationflag.New(
		s.watcherRegistry,
		authOK,
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Assert(err, tc.ErrorIsNil)

	results := facade.Phase(c.Context(), entities(
		coretesting.ModelTag.String(),
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 2)

	for _, result := range results.Results {
		c.Check(result.Error, tc.IsNil)
		c.Check(result.Phase, tc.Equals, "REAP")
	}
}

func (s *FacadeSuite) TestPhaseErrors(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	mig := modelmigration.Migration{}
	s.modelMigrationService.EXPECT().Migration(gomock.Any()).Return(mig, errors.New("ouch"))
	facade, err := migrationflag.New(
		s.watcherRegistry,
		authOK,
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Assert(err, tc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, call error.
	results := facade.Phase(c.Context(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 3)

	c.Check(results.Results, tc.DeepEquals, []params.PhaseResult{{
		Error: &params.Error{
			Message: `"urgle" is not a valid tag`,
		}}, {
		Error: &params.Error{
			Message: "permission denied",
			Code:    "unauthorized access",
		}}, {
		Error: &params.Error{
			Message: "ouch",
		},
	}})
}

func (s *FacadeSuite) TestWatchSuccess(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(ch)
	defer workertest.CleanKill(c, w)

	s.modelMigrationService.EXPECT().WatchMigrationPhase(gomock.Any()).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("123", nil)
	facade, err := migrationflag.New(
		s.watcherRegistry,
		authOK,
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Assert(err, tc.ErrorIsNil)

	results := facade.Watch(c.Context(), entities(
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 1)

	result := results.Results[0]
	c.Check(result.Error, tc.IsNil)
	c.Check(result.NotifyWatcherId, tc.Equals, "123")
}

func (s *FacadeSuite) TestWatchErrors(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.modelMigrationService.EXPECT().WatchMigrationPhase(gomock.Any()).Return(nil, errors.New("blort"))

	facade, err := migrationflag.New(
		s.watcherRegistry,
		authOK,
		s.modelAuthorisation(c),
		s.modelMigrationService)
	c.Assert(err, tc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, closed channel.
	results := facade.Watch(c.Context(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 3)

	c.Check(results.Results, tc.DeepEquals, []params.NotifyWatchResult{{
		Error: &params.Error{
			Message: `"urgle" is not a valid tag`,
		}}, {
		Error: &params.Error{
			Message: "permission denied",
			Code:    "unauthorized access",
		}}, {
		Error: &params.Error{
			Message: "blort",
		}},
	})
}
