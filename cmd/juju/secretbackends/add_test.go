// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apisecretbackends "github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type AddSuite struct {
	testhelpers.IsolationSuite
	store                *jujuclient.MemStore
	addSecretBackendsAPI *secretbackends.MockAddSecretBackendsAPI
}

func TestAddSuite(t *testing.T) {
	tc.Run(t, &AddSuite{})
}

func (s *AddSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *AddSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.addSecretBackendsAPI = secretbackends.NewMockAddSecretBackendsAPI(ctrl)

	return ctrl
}

func (s *AddSuite) TestAddInitError(c *tc.C) {
	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{},
		err:  "must specify backend name and type",
	}, {
		args: []string{"myvault", "vault"},
		err:  "must specify a config file or key values",
	}, {
		args: []string{"myvault", "somevault", "foo=bar"},
		err:  `invalid secret backend "somevault": no registered provider for "somevault"`,
	}, {
		args: []string{"myvault", "somevault", "foo=bar", "token-rotate=blah"},
		err:  `invalid token rotate interval: time: invalid duration "blah"`,
	}, {
		args: []string{"myvault", "somevault", "foo=bar", "token-rotate=1s"},
		err:  `token rotate interval "1s" less than 1h not valid`,
	}, {
		args: []string{"myvault", "somevault", "foo=bar", "token-rotate=0"},
		err:  `token rotate interval cannot be 0`,
	}, {
		args: []string{"myvault", "somevault", "foo=bar", "--config", "/path/to/nowhere"},
		err:  `open /path/to/nowhere: no such file or directory`,
	}} {
		_, err := cmdtesting.RunCommand(c, secretbackends.NewAddCommandForTest(s.store, s.addSecretBackendsAPI), t.args...)
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *AddSuite) TestAdd(c *tc.C) {
	defer s.setup(c).Finish()

	s.addSecretBackendsAPI.EXPECT().AddSecretBackend(
		gomock.Any(),
		apisecretbackends.CreateSecretBackend{
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
		}).Return(nil)
	s.addSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewAddCommandForTest(s.store, s.addSecretBackendsAPI),
		"myvault", "vault", "endpoint=http://vault", "token-rotate=666m",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *AddSuite) TestAddWithID(c *tc.C) {
	defer s.setup(c).Finish()

	s.addSecretBackendsAPI.EXPECT().AddSecretBackend(
		gomock.Any(),
		apisecretbackends.CreateSecretBackend{
			ID:          "backend-id",
			Name:        "myvault",
			BackendType: "vault",
			Config:      map[string]interface{}{"endpoint": "http://vault"},
		}).Return(nil)
	s.addSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewAddCommandForTest(s.store, s.addSecretBackendsAPI),
		"myvault", "vault", "endpoint=http://vault", "--import-id", "backend-id",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *AddSuite) TestAddFromFile(c *tc.C) {
	defer s.setup(c).Finish()

	fname := filepath.Join(c.MkDir(), "cfg.yaml")
	err := os.WriteFile(fname, []byte("endpoint: http://vault"), 0644)
	c.Assert(err, tc.ErrorIsNil)
	s.addSecretBackendsAPI.EXPECT().AddSecretBackend(
		gomock.Any(),
		apisecretbackends.CreateSecretBackend{
			Name:                "myvault",
			BackendType:         "vault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config: map[string]interface{}{
				"endpoint": "http://vault",
				"token":    "s.666",
			},
		}).Return(nil)
	s.addSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err = cmdtesting.RunCommand(c, secretbackends.NewAddCommandForTest(s.store, s.addSecretBackendsAPI),
		"myvault", "vault", "token=s.666", "token-rotate=666m", "--config", fname,
	)
	c.Assert(err, tc.ErrorIsNil)
}
