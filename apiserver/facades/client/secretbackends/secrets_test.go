// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/secretbackends"
	"github.com/juju/juju/apiserver/facades/client/secretbackends/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	clock        clock.Clock
	authorizer   *facademocks.MockAuthorizer
	backendState *mocks.MockSecretsBackendState
	secretsState *mocks.MockSecretsState
	statePool    *mocks.MockStatePool
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.backendState = mocks.NewMockSecretsBackendState(ctrl)
	s.secretsState = mocks.NewMockSecretsState(ctrl)
	s.statePool = mocks.NewMockStatePool(ctrl)

	s.clock = testclock.NewClock(time.Now())

	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecretBackendsIAAS(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeIAAS, nil, false)
}

func (s *SecretsSuite) TestListSecretBackendsFilterOnName(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeIAAS, []string{"myvault"}, false)
}

func (s *SecretsSuite) TestListSecretBackendsCAAS(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeCAAS, nil, false)
}

func (s *SecretsSuite) TestListSecretBackendsReveal(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeIAAS, nil, true)
}

func ptr[T any](v T) *T {
	return &v
}

type providerWithConfig struct {
	provider.ProviderConfig
	provider.SupportAuthRefresh
	provider.SecretBackendProvider
}

func (providerWithConfig) ConfigSchema() environschema.Fields {
	return environschema.Fields{
		"token": {
			Secret: true,
		},
	}
}

func (providerWithConfig) ConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"namespace": "foo",
	}
}

func (providerWithConfig) ValidateConfig(oldCfg, newCfg provider.ConfigAttrs) error {
	return nil
}

type mockModel struct {
	common.Model
	modelType state.ModelType
}

func (m *mockModel) Name() string {
	return "fred"
}

func (m *mockModel) Type() state.ModelType {
	return m.modelType
}

func (s *SecretsSuite) assertListSecretBackends(c *gc.C, modelType state.ModelType, names []string, reveal bool) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	}

	p := mocks.NewMockSecretBackendProvider(ctrl)
	p.EXPECT().Type().Return("vault")
	b := mocks.NewMockSecretsBackend(ctrl)
	b.EXPECT().Ping().Return(errors.New("ping error"))
	s.PatchValue(&commonsecrets.GetProvider, func(string) (provider.SecretBackendProvider, error) {
		return providerWithConfig{
			SecretBackendProvider: p,
		}, nil
	})

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	uuid := coretesting.ModelTag.Id()
	if modelType == state.ModelTypeCAAS {
		s.statePool.EXPECT().GetModel(uuid).Return(&mockModel{modelType: modelType}, func() bool { return true }, nil)
	}

	vaultConfig := map[string]interface{}{
		"endpoint": "http://vault",
		"token":    "s.ajehjdee",
	}
	p.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: "vault", Config: vaultConfig},
	}).Return(b, nil)

	backends := map[string]set.Strings{
		coretesting.ControllerTag.Id(): set.NewStrings("a"),
		"backend-id":                   set.NewStrings("b", "c", "d"),
		"backend-id-notfound":          set.NewStrings("z"),
	}
	if modelType == state.ModelTypeCAAS {
		backends[uuid] = set.NewStrings("e", "f")
	}
	s.secretsState.EXPECT().ListModelSecrets(true).Return(backends, nil)
	s.backendState.EXPECT().ListSecretBackends().Return(nil, nil)
	s.backendState.EXPECT().GetSecretBackendByID("backend-id").Return(&secrets.SecretBackend{
		ID:                  "backend-id",
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              vaultConfig,
	}, nil)
	s.backendState.EXPECT().GetSecretBackendByID("backend-id-notfound").Return(nil, errors.NotFoundf(""))

	results, err := facade.ListSecretBackends(context.Background(), params.ListSecretBackendsArgs{Names: names, Reveal: reveal})
	c.Assert(err, jc.ErrorIsNil)
	resultVaultCfg := map[string]interface{}{
		"endpoint": "http://vault",
		"token":    "s.ajehjdee",
	}
	if !reveal {
		delete(resultVaultCfg, "token")
	}
	wanted := set.NewStrings(names...)
	var backendResults []params.SecretBackendResult
	if wanted.IsEmpty() || wanted.Contains("myvault") {
		backendResults = []params.SecretBackendResult{{
			Result: params.SecretBackend{
				Name:                "myvault",
				BackendType:         "vault",
				TokenRotateInterval: ptr(666 * time.Minute),
				Config:              resultVaultCfg,
			},
			ID:         "backend-id",
			NumSecrets: 3,
			Status:     "error",
			Message:    "ping error",
		}}
	}
	if modelType == state.ModelTypeCAAS && (wanted.IsEmpty() || wanted.Contains("fred-local")) {
		backendResults = append(backendResults, params.SecretBackendResult{
			Result: params.SecretBackend{
				Name:        "fred-local",
				BackendType: "kubernetes",
				Config:      map[string]interface{}{},
			},
			ID:         coretesting.ModelTag.Id(),
			Status:     "active",
			NumSecrets: 2,
		})
	}
	if wanted.IsEmpty() || wanted.Contains("internal") {
		backendResults = append(backendResults, params.SecretBackendResult{
			Result: params.SecretBackend{
				Name:        "internal",
				BackendType: "controller",
				Config:      map[string]interface{}{},
			},
			ID:         coretesting.ControllerTag.Id(),
			Status:     "active",
			NumSecrets: 1,
		})
	}
	c.Assert(results, jc.DeepEquals, params.ListSecretBackendsResults{
		Results: backendResults,
	})
}

func (s *SecretsSuite) TestListSecretBackendsPermissionDeniedReveal(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecretBackends(context.Background(), params.ListSecretBackendsArgs{Reveal: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestAddSecretBackends(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	p := mocks.NewMockSecretBackendProvider(ctrl)
	p.EXPECT().Type().Return("vault").Times(2)
	b := mocks.NewMockSecretsBackend(ctrl)
	b.EXPECT().Ping().Return(nil).Times(2)
	s.PatchValue(&commonsecrets.GetProvider, func(pType string) (provider.SecretBackendProvider, error) {
		if pType != "vault" {
			return provider.Provider(pType)
		}
		return providerWithConfig{
			SecretBackendProvider: p,
		}, nil
	})

	addedConfig := map[string]interface{}{
		"endpoint":  "http://vault",
		"namespace": "foo",
	}
	p.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: "vault", Config: addedConfig},
	}).Return(b, nil).Times(2)

	s.backendState.EXPECT().CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(s.clock.Now().Add(150 * time.Minute)),
		Config:              addedConfig,
	}).Return("backend-id", nil)
	s.backendState.EXPECT().CreateSecretBackend(state.CreateSecretBackendParams{
		ID:          "existing-id",
		Name:        "myvault2",
		BackendType: "vault",
		Config:      addedConfig,
	}).Return("", errors.AlreadyExistsf(""))

	results, err := facade.AddSecretBackends(context.Background(), params.AddSecretBackendArgs{
		Args: []params.AddSecretBackendArg{{
			SecretBackend: params.SecretBackend{
				Name:                "myvault",
				BackendType:         "vault",
				TokenRotateInterval: ptr(200 * time.Minute),
				Config:              map[string]interface{}{"endpoint": "http://vault"},
			},
		}, {
			SecretBackend: params.SecretBackend{
				Name:        "invalid",
				BackendType: "something",
			},
		}, {
			ID: "existing-id",
			SecretBackend: params.SecretBackend{
				Name:        "myvault2",
				BackendType: "vault",
				Config:      map[string]interface{}{"endpoint": "http://vault"},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not found",
			Message: `creating backend provider type "something": no registered provider for "something"`}},
		{Error: &params.Error{
			Code:    "already exists",
			Message: `secret backend with ID "existing-id" already exists`}},
	})
}

func (s *SecretsSuite) TestAddSecretBackendsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.AddSecretBackends(context.Background(), params.AddSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretBackends(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	s.backendState.EXPECT().DeleteSecretBackend("myvault", true).Return(nil)
	s.backendState.EXPECT().DeleteSecretBackend("myvault2", false).Return(errors.NotSupportedf("remove with revisions"))

	results, err := facade.RemoveSecretBackends(context.Background(), params.RemoveSecretBackendArgs{
		Args: []params.RemoveSecretBackendArg{{
			Name:  "myvault",
			Force: true,
		}, {
			Name: "myvault2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not supported",
			Message: `remove with revisions not supported`}},
	})
}

func (s *SecretsSuite) TestRemoveSecretBackendsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.RemoveSecretBackends(context.Background(), params.RemoveSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestUpdateSecretBackends(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	p := mocks.NewMockSecretBackendProvider(ctrl)
	p.EXPECT().Type().Return("vault")
	b := mocks.NewMockSecretsBackend(ctrl)
	b.EXPECT().Ping().Return(nil)
	s.PatchValue(&commonsecrets.GetProvider, func(string) (provider.SecretBackendProvider, error) {
		return providerWithConfig{
			SecretBackendProvider: p,
		}, nil
	})

	updatedConfig := map[string]interface{}{
		"endpoint":        "http://vault",
		"namespace":       "foo",
		"tls-server-name": "server-name",
	}
	p.EXPECT().NewBackend(&provider.ModelBackendConfig{
		BackendConfig: provider.BackendConfig{BackendType: "vault", Config: updatedConfig},
	}).Return(b, nil)

	s.backendState.EXPECT().GetSecretBackend("myvault").Return(&secrets.SecretBackend{
		ID:          "backend-id",
		BackendType: "vault",
		Config:      map[string]interface{}{"endpoint": "http://vault"},
	}, nil)
	s.backendState.EXPECT().GetSecretBackend("invalid").Return(nil, errors.NotFoundf("backend"))

	s.backendState.EXPECT().UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  "backend-id",
		NameChange:          ptr("new-name"),
		TokenRotateInterval: ptr(200 * time.Minute),
		NextRotateTime:      ptr(s.clock.Now().Add(150 * time.Minute)),
		Config:              updatedConfig,
	}).Return(nil)

	results, err := facade.UpdateSecretBackends(context.Background(), params.UpdateSecretBackendArgs{
		Args: []params.UpdateSecretBackendArg{{
			Name:                "myvault",
			NameChange:          ptr("new-name"),
			TokenRotateInterval: ptr(200 * time.Minute),
			Config:              map[string]interface{}{"tls-server-name": "server-name"},
			Reset:               []string{"namespace"},
		}, {
			Name: "invalid",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not found",
			Message: `backend not found`}},
	})
}

func (s *SecretsSuite) TestUpdateSecretBackendsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.UpdateSecretBackends(context.Background(), params.UpdateSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
