// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"os"
	"path/filepath"

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

type updateSuite struct {
	jujutesting.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockUpdateSecretsAPI
}

var _ = gc.Suite(&updateSuite{})

func (s *updateSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *updateSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockUpdateSecretsAPI(ctrl)
	return ctrl
}

func (s *updateSuite) TestUpdateMissingArg(c *gc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(s.store, s.secretsAPI), "--label", "label", "--info", "this is a secret.")
	c.Assert(err, gc.ErrorMatches, `missing secret URI`)
}

func (s *updateSuite) TestUpdateWithoutContent(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(uri, ptr(true), "label", "this is a secret.", map[string]string{}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(),
		"--auto-prune", "--label", "label", "--info", "this is a secret.",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateFromArg(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(uri, ptr(true), "label", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	_, err := cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "foo=bar",
		"--auto-prune", "--label", "label", "--info", "this is a secret.",
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateSuite) TestUpdateFromFile(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().UpdateSecret(uri, ptr(true), "label", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	dir := c.MkDir()
	path := filepath.Join(dir, "data.txt")
	data := `
foo: bar
    `
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, secrets.NewUpdateCommandForTest(
		s.store, s.secretsAPI), uri.String(), "--file", path,
		"--auto-prune", "--label", "label", "--info", "this is a secret.",
	)
	c.Assert(err, jc.ErrorIsNil)
}
