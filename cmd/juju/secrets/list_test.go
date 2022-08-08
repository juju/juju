// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apisecrets "github.com/juju/juju/api/client/secrets"
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

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Revision: 2, Provider: "juju"},
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI:      uri2,
				Revision: 1, Provider: "juju"},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
URI                          Revision  Rotate  Backend  Age
%s         2  hourly  juju     0001-01-01  
%s         1  never   juju     0001-01-01  
`[1:], uri.ShortString(), uri2.ShortString()))
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(true).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, Revision: 2, Provider: "juju",
				Description: "my secret",
				OwnerTag:    "application-mysql",
				Label:       "foobar",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI: uri2, Version: 1, Revision: 1, Provider: "juju",
			},
			Error: "boom",
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "yaml", "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
- URI: %s
  version: 1
  owner: mysql
  backend: juju
  revision: 2
  description: my secret
  label: foobar
  rotate-policy: hourly
  create-time: 0001-01-01T00:00:00Z
  update-time: 0001-01-01T00:00:00Z
  value:
    foo: bar
- URI: %s
  version: 1
  backend: juju
  revision: 1
  create-time: 0001-01-01T00:00:00Z
  update-time: 0001-01-01T00:00:00Z
  error: boom
`[1:], uri.ShortString(), uri2.ShortString()))
}

func (s *ListSuite) TestListJSON(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(true).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI:     uri,
				Version: 1, Revision: 2, Provider: "juju",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "json", "--show-secrets")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
[{"URI":"%s","version":1,"backend":"juju","revision":2,"create-time":"0001-01-01T00:00:00Z","update-time":"0001-01-01T00:00:00Z","value":{"Data":{"foo":"bar"}}}]
`[1:], uri.ShortString()))
}
