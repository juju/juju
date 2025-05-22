// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/secrets"
	"github.com/juju/juju/cmd/juju/secrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type addSuite struct {
	testhelpers.IsolationSuite
	store      *jujuclient.MemStore
	secretsAPI *mocks.MockAddSecretsAPI
}

func TestAddSuite(t *testing.T) {
	tc.Run(t, &addSuite{})
}

func (s *addSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *addSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsAPI = mocks.NewMockAddSecretsAPI(ctrl)
	return ctrl
}

func (s *addSuite) TestAddDataFromArg(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().CreateSecret(gomock.Any(), "my-secret", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(uri.String(), nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewAddCommandForTest(s.store, s.secretsAPI), "my-secret", "foo=bar", "--info", "this is a secret.")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, uri.String()+"\n")
}

func (s *addSuite) TestAddDataFromFile(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsAPI.EXPECT().CreateSecret(gomock.Any(), "my-secret", "this is a secret.", map[string]string{"foo": "YmFy"}).Return(uri.String(), nil)
	s.secretsAPI.EXPECT().Close().Return(nil)

	dir := c.MkDir()
	path := filepath.Join(dir, "data.txt")
	data := `
foo: bar
    `
	err := os.WriteFile(path, []byte(data), 0644)
	c.Assert(err, tc.ErrorIsNil)

	ctx, err := cmdtesting.RunCommand(c, secrets.NewAddCommandForTest(s.store, s.secretsAPI), "my-secret", "--file", path, "--info", "this is a secret.")
	c.Assert(err, tc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, tc.Equals, uri.String()+"\n")
}

func (s *addSuite) TestAddEmptyData(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := cmdtesting.RunCommand(c, secrets.NewAddCommandForTest(s.store, s.secretsAPI), "my-secret", "--info", "this is a secret.")
	c.Assert(err, tc.ErrorMatches, `missing secret value or filename`)
}
