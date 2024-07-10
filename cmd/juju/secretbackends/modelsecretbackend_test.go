// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"github.com/juju/cmd/v4/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/jujuclient"
)

type modelSecretBackendCommandSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *secretbackends.MockModelSecretBackendAPI
}

var _ = gc.Suite(&modelSecretBackendCommandSuite{})

func (s *modelSecretBackendCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *modelSecretBackendCommandSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = secretbackends.NewMockModelSecretBackendAPI(ctrl)
	return ctrl
}

func (s *modelSecretBackendCommandSuite) TestGet(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().GetModelSecretBackend(gomock.Any()).Return("myVault", nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, "myVault"+"\n")
}

func (s *modelSecretBackendCommandSuite) TestSet(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().SetModelSecretBackend(gomock.Any(), "myVault").Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "myVault")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelSecretBackendCommandSuite) TestSetFailedEmptyBackendName(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "")
	c.Assert(err, gc.ErrorMatches, "cannot specify an empty secret backend name")
}

func (s *modelSecretBackendCommandSuite) TestSetFailedMoreThanOneArgs(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "foo", "bar")
	c.Assert(err, gc.ErrorMatches, "cannot specify multiple secret backend names")
}
