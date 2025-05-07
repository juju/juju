// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer      *facademocks.MockAuthorizer
	clock           clock.Clock
	watcherRegistry *facademocks.MockWatcherRegistry
	mockService     *MockBackendService
	mockWatcher     *MockSecretBackendRotateWatcher
	facade          *SecretBackendsManagerAPI
}

var _ = tc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.expectAuthController()
	s.clock = testclock.NewClock(time.Now())
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.mockService = NewMockBackendService(ctrl)
	s.mockWatcher = NewMockSecretBackendRotateWatcher(ctrl)

	var err error
	s.facade, err = NewTestAPI(s.authorizer, s.watcherRegistry, s.mockService, s.clock)
	c.Assert(err, tc.ErrorIsNil)
	return ctrl
}

func (s *SecretsManagerSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *SecretsManagerSuite) TestWatchBackendRotateChanges(c *tc.C) {
	defer s.setup(c).Finish()

	s.mockService.EXPECT().WatchSecretBackendRotationChanges(gomock.Any()).Return(s.mockWatcher, nil)
	s.watcherRegistry.EXPECT().Register(s.mockWatcher).Return("1", nil)

	next := time.Now().Add(time.Hour)
	rotateChan := make(chan []corewatcher.SecretBackendRotateChange, 1)
	rotateChan <- []corewatcher.SecretBackendRotateChange{{
		ID:              "backend-id",
		Name:            "myvault",
		NextTriggerTime: next,
	}}
	s.mockWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretBackendsRotateChanges(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SecretBackendRotateWatchResult{
		WatcherId: "1",
		Changes: []params.SecretBackendRotateChange{{
			ID:              "backend-id",
			Name:            "myvault",
			NextTriggerTime: next,
		}},
	})
}

func (s *SecretsManagerSuite) TestRotateBackendTokens(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.mockService.EXPECT().RotateBackendToken(gomock.Any(), "backend-id-1").Return(nil)
	s.mockService.EXPECT().RotateBackendToken(gomock.Any(), "backend-id-2").Return(errors.New("boom"))

	result, err := s.facade.RotateBackendTokens(context.Background(), params.RotateSecretBackendArgs{
		BackendIDs: []string{
			"backend-id-1",
			"backend-id-2",
		}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
		},
	})
}
