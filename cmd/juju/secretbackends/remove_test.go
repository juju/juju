// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jujuerrors "github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/cmd/juju/secretbackends/mocks"
	"github.com/juju/juju/jujuclient"
)

type RemoveSuite struct {
	jujutesting.IsolationSuite
	store                   *jujuclient.MemStore
	removeSecretBackendsAPI *mocks.MockRemoveSecretBackendsAPI
}

var _ = gc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *RemoveSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.removeSecretBackendsAPI = mocks.NewMockRemoveSecretBackendsAPI(ctrl)

	return ctrl
}

func (s *RemoveSuite) TestRemoveInitError(c *gc.C) {
	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{},
		err:  "must specify backend name",
	}, {
		args: []string{"myvault", "extra"},
		err:  `unrecognized args: \["extra"\]`,
	}} {
		_, err := cmdtesting.RunCommand(c, secretbackends.NewRemoveCommandForTest(s.store, s.removeSecretBackendsAPI), t.args...)
		c.Assert(err, gc.ErrorMatches, t.err)
	}
}

func (s *RemoveSuite) TestRemove(c *gc.C) {
	defer s.setup(c).Finish()

	s.removeSecretBackendsAPI.EXPECT().RemoveSecretBackend("myvault", true).Return(nil)
	s.removeSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewRemoveCommandForTest(s.store, s.removeSecretBackendsAPI),
		"myvault", "--force",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveSuite) TestRemoveNotSupported(c *gc.C) {
	defer s.setup(c).Finish()

	s.removeSecretBackendsAPI.EXPECT().RemoveSecretBackend("myvault", false).Return(
		jujuerrors.NotSupportedf(""))
	s.removeSecretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewRemoveCommandForTest(s.store, s.removeSecretBackendsAPI),
		"myvault",
	)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, `ERROR backend "myvault" still contains secret content\n`)
}
