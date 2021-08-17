// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apisecrets "github.com/juju/juju/api/secrets"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/jujuclient"
)

type ListSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockListSecretsAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *ListSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretsAPI = mocks.NewMockListSecretsAPI(ctrl)

	return ctrl
}

func (s *ListSuite) TestListTabular(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().ListSecrets(false).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				ID: 666, Scope: coresecrets.ScopeApplication,
				Revision: 2, Path: "app.password"},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
ID         Scope  Revision  Path
666  application  2         app.password  

`[1:])
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().ListSecrets(true).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				ID: 666, Scope: coresecrets.ScopeApplication,
				Version: 1, Revision: 2, Path: "app.password"},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "yaml", "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
- ID: 666
  revision: 2
  path: app.password
  scope: application
  version: 1
  create-time: 0001-01-01T00:00:00Z
  update-time: 0001-01-01T00:00:00Z
  value:
    foo: bar
`[1:])
}

func (s *ListSuite) TestListJSON(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().ListSecrets(true).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				ID: 666, Scope: coresecrets.ScopeApplication,
				Version: 1, Revision: 2, Path: "app.password"},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "json", "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
[{"ID":666,"revision":2,"path":"app.password","scope":"application","version":1,"create-time":"0001-01-01T00:00:00Z","update-time":"0001-01-01T00:00:00Z","value":{"Data":{"foo":"bar"}}}]
`[1:])
}
