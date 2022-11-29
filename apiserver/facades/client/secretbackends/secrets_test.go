// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/secretbackends"
	"github.com/juju/juju/apiserver/facades/client/secretbackends/mocks"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer   *facademocks.MockAuthorizer
	secretsState *mocks.MockSecretsBackendState
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretsState = mocks.NewMockSecretsBackendState(ctrl)

	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecretBackends(c *gc.C) {
	s.assertListSecretBackends(c, false)
}

func (s *SecretsSuite) TestListSecretBackendsReveal(c *gc.C) {
	s.assertListSecretBackends(c, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecretBackends(c *gc.C, reveal bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
			true, nil)
	}

	facade, err := secretbackends.NewTestAPI(s.secretsState, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	config := map[string]interface{}{"foo": "bar"}
	s.secretsState.EXPECT().ListSecretBackends().Return(
		[]*coresecrets.SecretBackend{{
			Name:                "myvault",
			Backend:             "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              config,
		}}, nil,
	)

	results, err := facade.ListSecretBackends(params.ListSecretBackendsArgs{Reveal: reveal})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretBackendsResults{
		Results: []params.SecretBackend{{
			Name:                "myvault",
			Backend:             "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              config,
		}},
	})
}

func (s *SecretsSuite) TestListSecretBackendsPermissionDeniedReveal(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		false, nil)

	facade, err := secretbackends.NewTestAPI(s.secretsState, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecretBackends(params.ListSecretBackendsArgs{Reveal: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestAddSecretBackends(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		true, nil)

	facade, err := secretbackends.NewTestAPI(s.secretsState, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	config := map[string]interface{}{"foo": "bar"}
	s.secretsState.EXPECT().CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                "myvault",
		Backend:             "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
	}).Return(nil)

	results, err := facade.AddSecretBackends(params.AddSecretBackendArgs{
		Args: []params.SecretBackend{{
			Name:                "myvault",
			Backend:             "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              config,
		}, {
			Name:    "invalid",
			Backend: "something",
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

	facade, err := secretbackends.NewTestAPI(s.secretsState, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.AddSecretBackends(params.AddSecretBackendArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
