// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
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

	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecretBackendsIAAS(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeIAAS, false)
}

func (s *SecretsSuite) TestListSecretBackendsCAAS(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeCAAS, false)
}

func (s *SecretsSuite) TestListSecretBackendsReveal(c *gc.C) {
	s.assertListSecretBackends(c, state.ModelTypeIAAS, true)
}

func ptr[T any](v T) *T {
	return &v
}

type providerWithConfig struct {
	provider.ProviderConfig
	provider.SecretBackendProvider
}

func (providerWithConfig) ConfigSchema() environschema.Fields {
	return environschema.Fields{
		"token": {
			Secret: true,
		},
	}
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

func (s *SecretsSuite) assertListSecretBackends(c *gc.C, modelType state.ModelType, reveal bool) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
			true, nil)
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

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
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

	results, err := facade.ListSecretBackends(params.ListSecretBackendsArgs{Reveal: reveal})
	c.Assert(err, jc.ErrorIsNil)
	resultVaultCfg := map[string]interface{}{
		"endpoint": "http://vault",
		"token":    "s.ajehjdee",
	}
	if !reveal {
		delete(resultVaultCfg, "token")
	}
	backendResults := []params.SecretBackendResult{{
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
	if modelType == state.ModelTypeCAAS {
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
	c.Assert(results, jc.DeepEquals, params.ListSecretBackendsResults{
		Results: backendResults,
	})
}

func (s *SecretsSuite) TestListSecretBackendsPermissionDeniedReveal(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		false, nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecretBackends(params.ListSecretBackendsArgs{Reveal: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestAddSecretBackends(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		true, nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	config := map[string]interface{}{"endpoint": "http://vault"}
	s.backendState.EXPECT().CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}).Return(nil)

	results, err := facade.AddSecretBackends(params.AddSecretBackendArgs{
		Args: []params.SecretBackend{{
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              config,
		}, {
			Name:        "invalid",
			BackendType: "something",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not found",
			Message: `creating backend provider type "something": no registered provider for "something"`}},
	})
}

func (s *SecretsSuite) TestAddSecretBackendsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		false, nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.AddSecretBackends(params.AddSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretBackends(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		true, nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	s.backendState.EXPECT().DeleteSecretBackend("myvault", true).Return(nil)
	s.backendState.EXPECT().DeleteSecretBackend("myvault2", false).Return(errors.NotSupportedf("remove with revisions"))

	results, err := facade.RemoveSecretBackends(params.RemoveSecretBackendArgs{
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
		false, nil)

	facade, err := secretbackends.NewTestAPI(s.backendState, s.secretsState, s.statePool, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.RemoveSecretBackends(params.RemoveSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
