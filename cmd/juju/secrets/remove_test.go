// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

type removeSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockRemoveSecretsAPI
}

var _ = tc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *removeSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockRemoveSecretsAPI(ctrl)
	return ctrl
}

func (s *removeSuite) TestRemoveMissingArg(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), "--revision", "4")
	c.Assert(err, tc.ErrorMatches, `missing secret URI`)
}

func (s *removeSuite) TestRemoveWithRevision(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RemoveSecret(gomock.Any(), uri, "", ptr(4)).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), uri.String(), "--revision", "4")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemove(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RemoveSecret(gomock.Any(), uri, "", nil).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), uri.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveByName(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsAPI.EXPECT().RemoveSecret(gomock.Any(), nil, "my-secret", nil).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), "my-secret")
	c.Assert(err, jc.ErrorIsNil)
}
