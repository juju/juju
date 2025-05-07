// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type updateSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockUpdateSecretsAPI
}

var _ = tc.Suite(&updateSuite{})

func (s *updateSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *updateSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockUpdateSecretsAPI(ctrl)
	return ctrl
}

func (s *updateSuite) TestUpdateMissingArg(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(s.store, s.secretsAPI), "--name", "new-name", "--info", "this is a secret.")
	c.Assert(err, tc.ErrorMatches, `missing secret URI`)
}

func (s *updateSuite) TestUpdateWithoutContent(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(gomock.Any(), uri, "", ptr(true), "new-name", "this is a secret.", map[string]string{}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(),
		"--auto-prune", "--name", "new-name", "--info", "this is a secret.",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateFromArg(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(gomock.Any(), uri, "", ptr(true), "new-name", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "foo=bar",
		"--auto-prune", "--name", "new-name", "--info", "this is a secret.",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateAutoPruneFalse(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(gomock.Any(), uri, "", ptr(false), "", "", map[string]string{}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "--auto-prune=false",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateAutoPruneNil(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(gomock.Any(), uri, "", nil, "", "this is a secret.", map[string]string{}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "--info", "this is a secret.",
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateFromFile(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(gomock.Any(), uri, "", ptr(true), "new-name", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	dir := c.MkDir()
	path := filepath.Join(dir, "data.txt")
	data := `
foo: bar
    `
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, tc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "--file", path,
		"--auto-prune", "--name", "new-name", "--info", "this is a secret.",
	)
	c.Assert(err, tc.ErrorIsNil)
}
