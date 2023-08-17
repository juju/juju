// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/secretbackendmanager"
	"github.com/juju/juju/apiserver/facades/controller/secretbackendmanager/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer      *facademocks.MockAuthorizer
	watcherRegistry *facademocks.MockWatcherRegistry

	provider             *mocks.MockSecretBackendProvider
	backendState         *mocks.MockBackendState
	backendRotate        *mocks.MockBackendRotate
	backendRotateWatcher *mocks.MockSecretBackendRotateWatcher
	clock                clock.Clock

	facade *secretbackendmanager.SecretBackendsManagerAPI
}

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.provider = mocks.NewMockSecretBackendProvider(ctrl)
	s.backendState = mocks.NewMockBackendState(ctrl)
	s.backendRotate = mocks.NewMockBackendRotate(ctrl)
	s.backendRotateWatcher = mocks.NewMockSecretBackendRotateWatcher(ctrl)
	s.expectAuthController()

	s.clock = testclock.NewClock(time.Now())

	var err error
	s.facade, err = secretbackendmanager.NewTestAPI(
		s.authorizer, s.watcherRegistry, s.backendState, s.backendRotate, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *SecretsManagerSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsManagerSuite) TestWatchBackendRotateChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.backendRotate.EXPECT().WatchSecretBackendRotationChanges().Return(
		s.backendRotateWatcher, nil,
	)
	s.watcherRegistry.EXPECT().Register(s.backendRotateWatcher).Return("1", nil)

	next := time.Now().Add(time.Hour)
	rotateChan := make(chan []corewatcher.SecretBackendRotateChange, 1)
	rotateChan <- []corewatcher.SecretBackendRotateChange{{
		ID:              "backend-id",
		Name:            "myvault",
		NextTriggerTime: next,
	}}
	s.backendRotateWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretBackendsRotateChanges(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretBackendRotateWatchResult{
		WatcherId: "1",
		Changes: []params.SecretBackendRotateChange{{
			ID:              "backend-id",
			Name:            "myvault",
			NextTriggerTime: next,
		}},
	})
}

type providerWithRefresh struct {
	provider.ProviderConfig
	provider.SupportAuthRefresh
	provider.SecretBackendProvider
}

func (providerWithRefresh) RefreshAuth(adminCfg *provider.ModelBackendConfig, validFor time.Duration) (*provider.BackendConfig, error) {
	result := *adminCfg
	result.Config["token"] = validFor.String()
	return &result.BackendConfig, nil
}

func (s *SecretsManagerSuite) TestRotateBackendTokens(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	backend := &coresecrets.SecretBackend{
		BackendType:         "vault",
		TokenRotateInterval: ptr(200 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	s.backendState.EXPECT().GetSecretBackendByID("backend-id").Return(backend, nil)

	p := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&secretbackendmanager.GetProvider, func(string) (provider.SecretBackendProvider, error) {
		return providerWithRefresh{
			SecretBackendProvider: p,
		}, nil
	})
	s.backendState.EXPECT().UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID: "backend-id",
		Config: map[string]interface{}{
			"foo":   "bar",
			"token": "3h20m0s",
		},
	}).Return(nil)

	nextRotateTime := s.clock.Now().Add(150 * time.Minute)
	s.backendState.EXPECT().SecretBackendRotated("backend-id", nextRotateTime).Return(errors.New("boom"))

	result, err := s.facade.RotateBackendTokens(context.Background(), params.RotateSecretBackendArgs{
		BackendIDs: []string{"backend-id"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestRotateBackendTokensRetry(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	backend := &coresecrets.SecretBackend{
		BackendType:         "vault",
		TokenRotateInterval: ptr(200 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	s.backendState.EXPECT().GetSecretBackendByID("backend-id").Return(backend, nil)

	p := mocks.NewMockSecretBackendProvider(ctrl)
	s.PatchValue(&secretbackendmanager.GetProvider, func(string) (provider.SecretBackendProvider, error) {
		return providerWithRefresh{
			SecretBackendProvider: p,
		}, nil
	})
	s.backendState.EXPECT().UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID: "backend-id",
		Config: map[string]interface{}{
			"foo":   "bar",
			"token": "3h20m0s",
		},
	}).Return(errors.New("BOOM"))

	// On error, try again after a short time.
	nextRotateTime := s.clock.Now().Add(2 * time.Minute)

	s.backendState.EXPECT().SecretBackendRotated("backend-id", nextRotateTime).Return(errors.New("boom"))

	result, err := s.facade.RotateBackendTokens(context.Background(), params.RotateSecretBackendArgs{
		BackendIDs: []string{"backend-id"}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
		},
	})
}
