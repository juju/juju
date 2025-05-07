// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type grantSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockGrantRevokeSecretsAPI
}

var _ = tc.Suite(&grantSuite{})

func (s *grantSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *grantSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockGrantRevokeSecretsAPI(ctrl)
	return ctrl
}

func (s *grantSuite) TestGrant(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().GrantSecret(gomock.Any(), uri, "", []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewGrantCommandForTest(s.store, s.secretsAPI), uri.String(), "gitlab,mysql")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *grantSuite) TestGrantByName(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().GrantSecret(gomock.Any(), nil, "my-secret", []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewGrantCommandForTest(s.store, s.secretsAPI), "my-secret", "gitlab,mysql")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *grantSuite) TestGrantEmptyData(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewGrantCommandForTest(s.store, s.secretsAPI), "arg1")
	c.Assert(err, tc.ErrorMatches, `missing secret URI or application name`)
}

type revokeSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockGrantRevokeSecretsAPI
}

var _ = tc.Suite(&revokeSuite{})

func (s *revokeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *revokeSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockGrantRevokeSecretsAPI(ctrl)
	return ctrl
}

func (s *revokeSuite) TestRevoke(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RevokeSecret(gomock.Any(), uri, "", []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRevokeCommandForTest(s.store, s.secretsAPI), uri.String(), "gitlab,mysql")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *revokeSuite) TestRevokeByName(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().RevokeSecret(gomock.Any(), nil, "my-secret", []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRevokeCommandForTest(s.store, s.secretsAPI), "my-secret", "gitlab,mysql")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *revokeSuite) TestRevokeEmptyData(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewRevokeCommandForTest(s.store, s.secretsAPI), "arg1")
	c.Assert(err, tc.ErrorMatches, `missing secret URI or application name`)
}
