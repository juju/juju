// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/jujuclient"
)

type removeSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockRemoveSecretsAPI
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *removeSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockRemoveSecretsAPI(ctrl)
	return ctrl
}

func (s *removeSuite) TestRemoveMissingArg(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), "--revision", "4")
	c.Assert(err, gc.ErrorMatches, `missing secret URI`)
}

func (s *removeSuite) TestRemoveWithRevision(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RemoveSecret(uri, ptr(4)).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), uri.String(), "--revision", "4")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemove(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().RemoveSecret(uri, nil).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewRemoveCommandForTest(s.store, s.secretsAPI), uri.String())
	c.Assert(err, jc.ErrorIsNil)
}
