// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"

	"github.com/juju/cmd/v4/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apisecrets "github.com/juju/juju/api/client/secrets"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
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
	uri3 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				LatestRevision: 2, OwnerTag: "application-mysql"},
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI:            uri2,
				LatestRevision: 1, OwnerTag: "application-mariadb"},
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI:            uri3,
				Label:          "my-secret",
				LatestRevision: 1, OwnerTag: coretesting.ModelTag.String()},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
ID                    Name       Owner    Rotation  Revision  Last updated
%s  my-secret  <model>  never            1  0001-01-01  
%s  -          mariadb  never            1  0001-01-01  
%s  -          mysql    hourly           2  0001-01-01  
`[1:], uri3.ID, uri2.ID, uri.ID))
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description: "my secret",
				OwnerTag:    "application-mysql",
				Label:       "foobar",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI: uri2, Version: 1, LatestRevision: 1, OwnerTag: "application-mariadb",
			},
			Error: "boom",
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI: uri3, Version: 1, LatestRevision: 1,
				Label: "my-secret", OwnerTag: coretesting.ModelTag.String(),
			},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "yaml")
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
%s:
  revision: 1
  owner: mariadb
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
  error: boom
%s:
  revision: 1
  owner: <model>
  name: my-secret
  created: 0001-01-01T00:00:00Z
  updated: 0001-01-01T00:00:00Z
`[1:], uri.ID, uri2.ID, uri3.ID))
}

func (s *ListSuite) TestListJSON(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI:     uri,
				Version: 1, LatestRevision: 2, OwnerTag: "application-mariadb",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
{"%s":{"revision":2,"owner":"mariadb","created":"0001-01-01T00:00:00Z","updated":"0001-01-01T00:00:00Z"}}
`[1:], uri.ID))
}
