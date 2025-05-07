// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apisecretbackends "github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

type UpdateSuite struct {
	jujutesting.IsolationSuite
	store                   *jujuclient.MemStore
	updateSecretBackendsAPI *secretbackends.MockUpdateSecretBackendsAPI
}

var _ = tc.Suite(&UpdateSuite{})

func (s *UpdateSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *UpdateSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.updateSecretBackendsAPI = secretbackends.NewMockUpdateSecretBackendsAPI(ctrl)

	return ctrl
}

func (s *UpdateSuite) TestUpdateInitError(c *tc.C) {
	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{},
		err:  "must specify backend name",
	}, {
		args: []string{"myvault"},
		err:  "must specify a config file or key/reset values",
	}, {
		args: []string{"myvault", "foo=bar", "token-rotate=1s"},
		err:  `token rotate interval "1s" less than 1h not valid`,
	}, {
		args: []string{"myvault", "foo=bar", "--config", "/path/to/nowhere"},
		err:  `open /path/to/nowhere: no such file or directory`,
	}} {
		_, err := cmdtesting.RunCommand(c, secretbackends.NewUpdateCommandForTest(s.store, s.updateSecretBackendsAPI), t.args...)
		c.Check(err, tc.ErrorMatches, t.err)
	}
}

func (s *UpdateSuite) TestUpdate(c *tc.C) {
	defer s.setup(c).Finish()

	s.updateSecretBackendsAPI.EXPECT().UpdateSecretBackend(
		gomock.Any(),
		apisecretbackends.UpdateSecretBackend{
			Name:                "myvault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
		}, true).Return(nil)
	s.updateSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewUpdateCommandForTest(s.store, s.updateSecretBackendsAPI),
		"myvault", "endpoint=http://vault", "token-rotate=666m", "--force",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpdateSuite) TestUpdateName(c *tc.C) {
	defer s.setup(c).Finish()

	s.updateSecretBackendsAPI.EXPECT().UpdateSecretBackend(
		gomock.Any(),
		apisecretbackends.UpdateSecretBackend{
			Name:       "myvault",
			NameChange: ptr("myvault2"),
			Config:     map[string]interface{}{"endpoint": "http://vault"},
		}, false).Return(nil)
	s.updateSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewUpdateCommandForTest(s.store, s.updateSecretBackendsAPI),
		"myvault", "endpoint=http://vault", "name=myvault2",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpdateSuite) TestUpdateResetTokenRotate(c *tc.C) {
	defer s.setup(c).Finish()

	s.updateSecretBackendsAPI.EXPECT().UpdateSecretBackend(
		gomock.Any(),
		apisecretbackends.UpdateSecretBackend{
			Name:                "myvault",
			TokenRotateInterval: ptr(0 * time.Second),
			Config:              map[string]interface{}{"endpoint": "http://vault"},
		}, false).Return(nil)
	s.updateSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewUpdateCommandForTest(s.store, s.updateSecretBackendsAPI),
		"myvault", "endpoint=http://vault", "token-rotate=0",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpdateSuite) TestUpdateFromFile(c *tc.C) {
	defer s.setup(c).Finish()

	fname := filepath.Join(c.MkDir(), "cfg.yaml")
	err := os.WriteFile(fname, []byte("endpoint: http://vault"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.updateSecretBackendsAPI.EXPECT().UpdateSecretBackend(
		gomock.Any(),
		apisecretbackends.UpdateSecretBackend{
			Name:                "myvault",
			TokenRotateInterval: ptr(666 * time.Minute),
			Config: map[string]interface{}{
				"endpoint": "http://vault",
				"token":    "s.666",
			},
		}, false).Return(nil)
	s.updateSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err = cmdtesting.RunCommand(c, secretbackends.NewUpdateCommandForTest(s.store, s.updateSecretBackendsAPI),
		"myvault", "token=s.666", "token-rotate=666m", "--config", fname,
	)
	c.Assert(err, jc.ErrorIsNil)
}
