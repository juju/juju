// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type MigrateSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api                 *fakeMigrateAPI
	targetControllerAPI *fakeTargetControllerAPI
	modelAPI            *fakeModelAPI
	store               *jujuclient.MemStore
	password            string
}

var _ = gc.Suite(&MigrateSuite{})

const modelUUID = "deadbeef-0bad-400d-8000-4b1d0d06f00d"
const targetControllerUUID = "beefdead-0bad-400d-8000-4b1d0d06f00d"

func (s *MigrateSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()

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
		User: "sourceuser",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Define the account for the target controller.
	err = s.store.UpdateAccount("target", jujuclient.AccountDetails{
		User:     "targetuser",
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
	s.modelAPI = &fakeModelAPI{
		models: []base.UserModel{{
			Name:  "model",
			UUID:  modelUUID,
			Type:  model.IAAS,
			Owner: "sourceuser",
		}, {
			Name:  "production",
			UUID:  "prod-1-uuid",
			Type:  model.IAAS,
			Owner: "alpha",
		}, {
			Name:  "production",
			UUID:  "prod-2-uuid",
			Type:  model.IAAS,
			Owner: "sourceuser",
		}},
	}

	mac0, err := macaroon.New([]byte("secret0"), []byte("id0"), "location0")
	c.Assert(err, jc.ErrorIsNil)
	mac1, err := macaroon.New([]byte("secret1"), []byte("id1"), "location1")
	c.Assert(err, jc.ErrorIsNil)

	jar, err := s.store.CookieJar("target")
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

}

func addCookie(c *gc.C, jar http.CookieJar, mac *macaroon.Macaroon, url *url.URL) {
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

	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen, jc.DeepEquals, &controller.MigrationSpec{
		ModelUUID:            modelUUID,
		TargetControllerUUID: targetControllerUUID,
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "targetuser",
		TargetPassword:       "secret",
	})
}

func (s *MigrateSuite) TestSuccessMacaroons(c *gc.C) {
	err := s.store.UpdateAccount("target", jujuclient.AccountDetails{
		User:     "targetuser",
		Password: "",
	})
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.makeAndRun(c, "model", "target")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	// Extract macaroons so we can compare them separately
	// (as they can't be compared using DeepEquals due to 'UnmarshaledAs')
	macs := s.api.specSeen.TargetMacaroons
	s.api.specSeen.TargetMacaroons = nil
	apitesting.MacaroonsEqual(c, macs, s.targetControllerAPI.macaroons)
	c.Check(s.api.specSeen, jc.DeepEquals, &controller.MigrationSpec{
		ModelUUID:            modelUUID,
		TargetControllerUUID: targetControllerUUID,
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "targetuser",
	})
}

func (s *MigrateSuite) TestModelDoesntExist(c *gc.C) {
	cmd := s.makeCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "wat", "target")
	c.Check(err, gc.ErrorMatches, "model .+ not found")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) TestMultipleModelMatch(c *gc.C) {
	cmd := s.makeCommand()
	// Disambiguation is done in the standard way by choosing
	// the current user's model.
	ctx, err := cmdtesting.RunCommand(c, cmd, "production", "target")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen, jc.DeepEquals, &controller.MigrationSpec{
		ModelUUID:            "prod-2-uuid",
		TargetControllerUUID: targetControllerUUID,
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "targetuser",
		TargetPassword:       "secret",
	})
}

func (s *MigrateSuite) TestSpecifyOwner(c *gc.C) {
	ctx, err := s.makeAndRun(c, "alpha/production", "target")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen.ModelUUID, gc.Equals, "prod-1-uuid")
}

func (s *MigrateSuite) TestControllerDoesntExist(c *gc.C) {
	_, err := s.makeAndRun(c, "model", "wat")
	c.Check(err, gc.ErrorMatches, "controller wat not found")
	c.Check(s.api.specSeen, gc.IsNil) // API shouldn't have been called
}

func (s *MigrateSuite) makeAndRun(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.makeCommand(), args...)
}

func (s *MigrateSuite) makeCommand() modelcmd.ModelCommand {
	cmd := newMigrateCommand()
	cmd.SetClientStore(s.store)
	cmd.SetModelAPI(s.modelAPI)
	inner := modelcmd.InnerCommand(cmd).(*migrateCommand)
	inner.migAPI = s.api
	inner.newAPIRoot = func(jujuclient.ClientStore, string, string) (api.Connection, error) {
		return s.targetControllerAPI, nil
	}
	return cmd
}

type fakeMigrateAPI struct {
	specSeen *controller.MigrationSpec
}

func (a *fakeMigrateAPI) InitiateMigration(spec controller.MigrationSpec) (string, error) {
	a.specSeen = &spec
	return "uuid:0", nil
}

func (*fakeMigrateAPI) Close() error {
	return nil
}

type fakeModelAPI struct {
	models []base.UserModel
}

func (m *fakeModelAPI) ListModels(user string) ([]base.UserModel, error) {
	return m.models, nil
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
