// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apisecretbackends "github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type ShowSuite struct {
	testhelpers.IsolationSuite
	store             *jujuclient.MemStore
	secretBackendsAPI *secretbackends.MockListSecretBackendsAPI
}

var _ = tc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *ShowSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretBackendsAPI = secretbackends.NewMockListSecretBackendsAPI(ctrl)

	return ctrl
}

func (s *ShowSuite) TestShowYAML(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendsAPI.EXPECT().ListSecretBackends(gomock.Any(), []string{"myvault"}, true).Return(
		[]apisecretbackends.SecretBackend{{
			ID:                  "vault-id",
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
			NumSecrets:          666,
			Status:              status.Error,
			Message:             "vault is sealed",
		}}, nil)

	s.secretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewShowCommandForTest(s.store, s.secretBackendsAPI), "myvault", "--reveal")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, `
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
