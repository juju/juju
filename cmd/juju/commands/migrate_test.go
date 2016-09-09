// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/cmd"
	cookiejar "github.com/juju/persistent-cookiejar"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type MigrateSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api                 *fakeMigrateAPI
	targetControllerAPI *fakeTargetControllerAPI
	store               *jujuclienttesting.MemStore
	password            string
}

var _ = gc.Suite(&MigrateSuite{})

const modelUUID = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
const targetControllerUUID = "beefdead-0bad-400d-8000-4b1d0d06f00d"

func (s *MigrateSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.Migration)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclienttesting.NewMemStore()

	// Define the source controller in the config and set it as the default.
	err := s.store.AddController("source", jujuclient.ControllerDetails{
		ControllerUUID: "eeeeeeee-0bad-400d-8000-4b1d0d06f00d",
		CACert:         "somecert",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentController("source")
	c.Assert(err, jc.ErrorIsNil)

	// Define an account for the model in the source controller in the config.
	err = s.store.UpdateAccount("source", jujuclient.AccountDetails{
		User: "source@local",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Define the model to migrate in the config.
	err = s.store.UpdateModel("source", "source@local/model", jujuclient.ModelDetails{
		ModelUUID: modelUUID,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Define the account for the target controller.
	err = s.store.UpdateAccount("target", jujuclient.AccountDetails{
		User:     "target@local",
		Password: "secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Define the target controller in the config.
	err = s.store.AddController("target", jujuclient.ControllerDetails{
		ControllerUUID: targetControllerUUID,
		APIEndpoints:   []string{"1.2.3.4:5"},
		CACert:         "cert",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.api = &fakeMigrateAPI{}

	mac0, err := macaroon.New([]byte("secret0"), "id0", "location0")
	c.Assert(err, jc.ErrorIsNil)
	mac1, err := macaroon.New([]byte("secret1"), "id1", "location1")
	c.Assert(err, jc.ErrorIsNil)

	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: cookiejar.DefaultCookieFile(),
	})
	c.Assert(err, jc.ErrorIsNil)

	s.targetControllerAPI = &fakeTargetControllerAPI{
		cookieURL: &url.URL{
			Scheme: "https",
			Host:   "testing.invalid",
			Path:   "/",
		},
		macaroons: []macaroon.Slice{{mac0}},
	}
	addCookie(c, jar, mac0, s.targetControllerAPI.cookieURL)
	addCookie(c, jar, mac1, &url.URL{
		Scheme: "https",
		Host:   "tasting.invalid",
		Path:   "/",
	})

	err = jar.Save()
	c.Assert(err, jc.ErrorIsNil)
}

func addCookie(c *gc.C, jar *cookiejar.Jar, mac *macaroon.Macaroon, url *url.URL) {
	cookie, err := httpbakery.NewCookie(macaroon.Slice{mac})
	c.Assert(err, jc.ErrorIsNil)
	cookie.Expires = time.Now().Add(time.Hour) // only persistent cookies are stored
	jar.SetCookies(url, []*http.Cookie{cookie})
}

func (s *MigrateSuite) TestMissingModel(c *gc.C) {
	_, err := s.makeAndRun(c)
	c.Assert(err, gc.ErrorMatches, "model not specified")
}

func (s *MigrateSuite) TestMissingTargetController(c *gc.C) {
	_, err := s.makeAndRun(c, "mymodel")
	c.Assert(err, gc.ErrorMatches, "target controller not specified")
}

func (s *MigrateSuite) TestTooManyArgs(c *gc.C) {
	_, err := s.makeAndRun(c, "one", "too", "many")
	c.Assert(err, gc.ErrorMatches, "too many arguments specified")
}

func (s *MigrateSuite) TestSuccess(c *gc.C) {
	ctx, err := s.makeAndRun(c, "model", "target")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen, jc.DeepEquals, &controller.MigrationSpec{
		ModelUUID:            modelUUID,
		TargetControllerUUID: targetControllerUUID,
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "target@local",
		TargetPassword:       "secret",
	})
}

func (s *MigrateSuite) TestSuccessMacaroons(c *gc.C) {
	err := s.store.UpdateAccount("target", jujuclient.AccountDetails{
		User:     "target@local",
		Password: "",
	})
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.makeAndRun(c, "model", "target")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(testing.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen, jc.DeepEquals, &controller.MigrationSpec{
		ModelUUID:            modelUUID,
		TargetControllerUUID: targetControllerUUID,
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "target@local",
		TargetMacaroons:      s.targetControllerAPI.macaroons,
	})
}

func (s *MigrateSuite) TestModelDoesntExist(c *gc.C) {
	cmd := s.makeCommand()
	cmd.SetModelApi(&fakeModelAPI{})
	_, err := s.run(c, cmd, "wat", "target")
	c.Check(err, gc.ErrorMatches, "model .+ not found")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) TestModelDoesntExistBeforeRefresh(c *gc.C) {
	cmd := s.makeCommand()
	cmd.SetModelApi(&fakeModelAPI{model: "wat"}) // Model is available after refresh
	_, err := s.run(c, cmd, "wat", "target")
	c.Check(err, jc.ErrorIsNil)
	c.Check(s.api.specSeen, gc.NotNil)
}

func (s *MigrateSuite) TestControllerDoesntExist(c *gc.C) {
	_, err := s.makeAndRun(c, "model", "wat")
	c.Check(err, gc.ErrorMatches, "controller wat not found")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) makeAndRun(c *gc.C, args ...string) (*cmd.Context, error) {
	return s.run(c, s.makeCommand(), args...)
}

func (s *MigrateSuite) makeCommand() *migrateCommand {
	cmd := &migrateCommand{
		api: s.api,
		newAPIRoot: func(jujuclient.ClientStore, string, string) (api.Connection, error) {
			return s.targetControllerAPI, nil
		},
	}
	cmd.SetClientStore(s.store)
	return cmd
}

func (s *MigrateSuite) run(c *gc.C, cmd *migrateCommand, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, modelcmd.WrapController(cmd), args...)
}

type fakeMigrateAPI struct {
	specSeen *controller.MigrationSpec
}

func (a *fakeMigrateAPI) InitiateMigration(spec controller.MigrationSpec) (string, error) {
	a.specSeen = &spec
	return "uuid:0", nil
}

type fakeModelAPI struct {
	model string
}

func (m *fakeModelAPI) ListModels(user string) ([]base.UserModel, error) {
	if m.model == "" {
		return []base.UserModel{}, nil
	}
	return []base.UserModel{{
		Name:  m.model,
		UUID:  modelUUID,
		Owner: "source@local",
	}}, nil
}

func (m *fakeModelAPI) Close() error {
	return nil
}

type fakeTargetControllerAPI struct {
	api.Connection
	cookieURL *url.URL
	macaroons []macaroon.Slice
}

func (a *fakeTargetControllerAPI) CookieURL() *url.URL {
	return a.cookieURL
}

func (a *fakeTargetControllerAPI) Close() error {
	return nil
}
