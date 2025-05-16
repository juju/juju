// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secretbackends"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type modelSecretBackendCommandSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *secretbackends.MockModelSecretBackendAPI
}

func TestModelSecretBackendCommandSuite(t *stdtesting.T) {
	tc.Run(t, &modelSecretBackendCommandSuite{})
}
func (s *modelSecretBackendCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *modelSecretBackendCommandSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = secretbackends.NewMockModelSecretBackendAPI(ctrl)
	return ctrl
}

func (s *modelSecretBackendCommandSuite) TestGet(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().GetModelSecretBackend(gomock.Any()).Return("myVault", nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, "myVault"+"\n")
}

func (s *modelSecretBackendCommandSuite) TestGetNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().GetModelSecretBackend(gomock.Any()).Return("", errors.NotSupportedf("model secret backend"))
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, tc.ErrorMatches, `"model-secret-backend" has not been implemented on the controller, use the "model-config" command instead`)
}

func (s *modelSecretBackendCommandSuite) TestSet(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().SetModelSecretBackend(gomock.Any(), "myVault").Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "myVault")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendCommandSuite) TestSetNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().SetModelSecretBackend(gomock.Any(), "myVault").Return(errors.NotSupportedf("model secret backend"))
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "myVault")
	c.Assert(err, tc.ErrorMatches, `"model-secret-backend" has not been implemented on the controller, use the "model-config" command instead`)
}

func (s *modelSecretBackendCommandSuite) TestSetSecretBackendNotFound(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().SetModelSecretBackend(gomock.Any(), "myVault").Return(secretbackenderrors.NotFound)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "myVault")
	c.Assert(err, tc.ErrorMatches, `secret backend not found: please use "add-secret-backend" to add "myVault" to the controller first`)
}

func (s *modelSecretBackendCommandSuite) TestSetSecretBackendNotValid(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().SetModelSecretBackend(gomock.Any(), "internal").Return(secretbackenderrors.NotValid)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "internal")
	c.Assert(err, tc.ErrorMatches, `secret backend not valid: please use "auto" instead`)
}

func (s *modelSecretBackendCommandSuite) TestSetFailedMoreThanOneArgs(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secretbackends.NewModelCredentialCommandForTest(s.store, s.secretsAPI), "foo", "bar")
	c.Assert(err, tc.ErrorMatches, "cannot specify multiple secret backend names")
}
