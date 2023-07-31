// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	// apisecrets "github.com/juju/juju/api/client/secrets"
	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/jujuclient"
)

type grantSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockGrantRevokeSecretsAPI
}

var _ = gc.Suite(&grantSuite{})

func (s *grantSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *grantSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockGrantRevokeSecretsAPI(ctrl)
	return ctrl
}

func (s *grantSuite) TestGrant(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().GrantSecret(uri, []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewGrantCommandForTest(s.store, s.secretsAPI), uri.String(), "gitlab,mysql")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *grantSuite) TestGrantEmptyData(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewGrantCommandForTest(s.store, s.secretsAPI), "arg1")
	c.Assert(err, gc.ErrorMatches, `missing secret URI or application name`)
}

type revokeSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockGrantRevokeSecretsAPI
}

var _ = gc.Suite(&revokeSuite{})

func (s *revokeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *revokeSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockGrantRevokeSecretsAPI(ctrl)
	return ctrl
}

func (s *revokeSuite) TestRevoke(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RevokeSecret(uri, []string{"gitlab", "mysql"}).Return([]error{nil, nil}, nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRevokeCommandForTest(s.store, s.secretsAPI), uri.String(), "gitlab,mysql")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *revokeSuite) TestRevokeEmptyData(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewRevokeCommandForTest(s.store, s.secretsAPI), "arg1")
	c.Assert(err, gc.ErrorMatches, `missing secret URI or application name`)
}
