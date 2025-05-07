// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secretbackends"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
)

type RemoveSuite struct {
	jujutesting.IsolationSuite
	store                   *jujuclient.MemStore
	removeSecretBackendsAPI *secretbackends.MockRemoveSecretBackendsAPI
}

var _ = tc.Suite(&RemoveSuite{})

func (s *RemoveSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *RemoveSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.removeSecretBackendsAPI = secretbackends.NewMockRemoveSecretBackendsAPI(ctrl)

	return ctrl
}

func (s *RemoveSuite) TestRemoveInitError(c *tc.C) {
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
		c.Assert(err, tc.ErrorMatches, t.err)
	}
}

func (s *RemoveSuite) TestRemove(c *tc.C) {
	defer s.setup(c).Finish()

	s.removeSecretBackendsAPI.EXPECT().RemoveSecretBackend(gomock.Any(), "myvault", true).Return(nil)
	s.removeSecretBackendsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secretbackends.NewRemoveCommandForTest(s.store, s.removeSecretBackendsAPI),
		"myvault", "--force",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveSuite) TestRemoveNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	s.removeSecretBackendsAPI.EXPECT().RemoveSecretBackend(gomock.Any(), "myvault", false).Return(
		jujuerrors.NotSupportedf(""))
	s.removeSecretBackendsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secretbackends.NewRemoveCommandForTest(s.store, s.removeSecretBackendsAPI),
		"myvault",
	)
	c.Assert(err, tc.Equals, cmd.ErrSilent)
	c.Check(cmdtesting.Stderr(ctx), tc.Matches, `ERROR backend "myvault" still contains secret content\n`)
}
