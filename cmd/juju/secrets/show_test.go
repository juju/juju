// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"

	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apisecrets "github.com/juju/juju/api/client/secrets"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ShowSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockListSecretsAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *ShowSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretsAPI = mocks.NewMockListSecretsAPI(ctrl)

	return ctrl
}

func (s *ShowSuite) TestInit(c *gc.C) {
	uri := coresecrets.NewURI()
	_, err := cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID, "--revisions", "--reveal")
	c.Assert(err, gc.ErrorMatches, "specify either --revisions or --reveal but not both")
	_, err = cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID, "--revisions", "--revision", "2")
	c.Assert(err, gc.ErrorMatches, "specify either --revisions or --revision but not both")
	_, err = cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID, "--revisions", "--revision", "-1")
	c.Assert(err, gc.ErrorMatches, "revision must be a positive integer")
}

func ptr[T any](v T) *T {
	return &v
}

func (s *ShowSuite) TestShow(c *gc.C) {
	defer s.setup(c).Finish()

	expire := testing.NonZeroTime().UTC()
	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{
		URI: uri,
	}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description:      "my secret",
				OwnerTag:         "application-mysql",
				Label:            "foobar",
				LatestExpireTime: &expire,
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
			Access: []coresecrets.AccessInfo{
				{
					Target: "application-gitlab",
					Scope:  "relation-key",
					Role:   coresecrets.RoleView,
				},
			},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID)
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
%s:
  revision: 2
  expires: 1970-01-01T00:00:00.000000001Z
  rotation: hourly
  owner: mysql
  description: my secret
  label: foobar
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
  access:
  - target: application-gitlab
    scope: relation-key
    role: view
`[1:], uri.ID))
}

func (s *ShowSuite) TestShowByName(c *gc.C) {
	defer s.setup(c).Finish()

	expire := testing.NonZeroTime().UTC()
	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{
		Label: ptr("my-secret"),
	}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description:      "my secret",
				OwnerTag:         "application-mysql",
				Label:            "foobar",
				LatestExpireTime: &expire,
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), "my-secret")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
%s:
  revision: 2
  expires: 1970-01-01T00:00:00.000000001Z
  rotation: hourly
  owner: mysql
  description: my secret
  label: foobar
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
`[1:], uri.ID))
}

func (s *ShowSuite) TestShowReveal(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(true, coresecrets.Filter{
		URI: uri,
	}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description: "my secret",
				OwnerTag:    "application-mysql",
				Label:       "foobar",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID, "--reveal")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
%s:
  revision: 2
  rotation: hourly
  owner: mysql
  description: my secret
  label: foobar
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
  content:
    foo: bar
`[1:], uri.ID))
}

func (s *ShowSuite) TestShowRevisions(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{
		URI: uri,
	}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description: "my secret",
				OwnerTag:    "application-mysql",
				Label:       "foobar",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
			Revisions: []coresecrets.SecretRevisionMetadata{{
				Revision:    666,
				BackendName: ptr("some backend"),
			}},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewShowCommandForTest(s.store, s.secretsAPI), uri.ID, "--revisions")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
%s:
  revision: 2
  rotation: hourly
  owner: mysql
  description: my secret
  label: foobar
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
  revisions:
  - revision: 666
    backend: some backend
    created: 0001-01-01T00:00:00Z
    updated: 0001-01-01T00:00:00Z
`[1:], uri.ID))
}
