// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apisecretbackends "github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ListSuite struct {
	testhelpers.IsolationSuite
	store             *jujuclient.MemStore
	secretBackendsAPI *secretbackends.MockListSecretBackendsAPI
}

var _ = tc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *ListSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretBackendsAPI = secretbackends.NewMockListSecretBackendsAPI(ctrl)

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *ListSuite) TestListTabular(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendsAPI.EXPECT().ListSecretBackends(gomock.Any(), nil, false).Return(
		[]apisecretbackends.SecretBackend{{
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
			NumSecrets:          666,
			Status:              status.Error,
			Message:             "vault is sealed",
		}, {
			Name:        "internal",
			BackendType: "controller",
			NumSecrets:  668,
			Status:      status.Active,
		}, {
			ID:    "backend-error-id",
			Error: errors.New("error"),
		}}, nil)
	s.secretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewListCommandForTest(s.store, s.secretBackendsAPI))
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
Name      Type        Secrets  Message
internal  controller  668                              
myvault   vault       666      error: vault is sealed  
`[1:])
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendsAPI.EXPECT().ListSecretBackends(gomock.Any(), nil, true).Return(
		[]apisecretbackends.SecretBackend{{
			ID:                  "vault-id",
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
			NumSecrets:          666,
			Status:              status.Error,
			Message:             "vault is sealed",
		}, {
			ID:          coretesting.ControllerTag.Id(),
			Name:        "internal",
			BackendType: "controller",
			NumSecrets:  668,
			Status:      status.Active,
		}, {
			ID:    "999",
			Error: errors.New("some error"),
		}}, nil)

	s.secretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewListCommandForTest(s.store, s.secretBackendsAPI), "--reveal", "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
error-999:
  secrets: 0
  status: error
  id: "999"
  error: some error
internal:
  backend: controller
  secrets: 668
  status: active
myvault:
  backend: vault
  token-rotate-interval: 11h6m0s
  config:
    endpoint: http://vault
  secrets: 666
  status: error
  message: vault is sealed
  id: vault-id
`[1:])
}

func (s *ListSuite) TestListJSON(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendsAPI.EXPECT().ListSecretBackends(gomock.Any(), nil, true).Return(
		[]apisecretbackends.SecretBackend{{
			Name:        "internal",
			BackendType: "controller",
			NumSecrets:  668,
			Status:      status.Active,
		}}, nil)
	s.secretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewListCommandForTest(s.store, s.secretBackendsAPI), "--reveal", "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
{"internal":{"backend":"controller","secrets":668,"status":"active"}}
`[1:])
}
