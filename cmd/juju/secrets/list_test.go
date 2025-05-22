// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apisecrets "github.com/juju/juju/api/client/secrets"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type ListSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockListSecretsAPI
}

func TestListSuite(t *stdtesting.T) {
	tc.Run(t, &ListSuite{})
}

func (s *ListSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *ListSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretsAPI = mocks.NewMockListSecretsAPI(ctrl)

	return ctrl
}

func (s *ListSuite) TestListTabular(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(gomock.Any(), false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				LatestRevision: 2, Owner: coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"}},
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI:            uri2,
				LatestRevision: 1, Owner: coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mariadb"}},
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI:            uri3,
				Label:          "my-secret",
				LatestRevision: 1, Owner: coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: coretesting.ModelTag.Id()}},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI))
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, fmt.Sprintf(`
ID                    Name       Owner    Rotation  Revision  Last updated
%s  my-secret  <model>  never            1  0001-01-01  
%s  -          mariadb  never            1  0001-01-01  
%s  -          mysql    hourly           2  0001-01-01  
`[1:], uri3.ID, uri2.ID, uri.ID))
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(gomock.Any(), false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI: uri, RotatePolicy: coresecrets.RotateHourly,
				Version: 1, LatestRevision: 2,
				Description: "my secret",
				Owner:       coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"},
				Label:       "foobar",
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI: uri2, Version: 1, LatestRevision: 1, Owner: coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mariadb"},
			},
			Error: "boom",
		}, {
			Metadata: coresecrets.SecretMetadata{
				URI: uri3, Version: 1, LatestRevision: 1,
				Label: "my-secret", Owner: coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: coretesting.ModelTag.Id()},
			},
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, fmt.Sprintf(`
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

func (s *ListSuite) TestListJSON(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().ListSecrets(gomock.Any(), false, coresecrets.Filter{}).Return(
		[]apisecrets.SecretDetails{{
			Metadata: coresecrets.SecretMetadata{
				URI:     uri,
				Version: 1, LatestRevision: 2, Owner: coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mariadb"},
			},
			Value: coresecrets.NewSecretValue(map[string]string{"foo": "YmFy"}),
		}}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewListCommandForTest(s.store, s.secretsAPI), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, fmt.Sprintf(`
{"%s":{"revision":2,"owner":"mariadb","created":"0001-01-01T00:00:00Z","updated":"0001-01-01T00:00:00Z"}}
`[1:], uri.ID))
}
