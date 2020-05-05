// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"net/http"
	"net/url"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
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
	userAPI             *fakeUserAPI
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

	userList := []params.ModelUserInfo{
		{
			UserName:    "admin",
			DisplayName: "admin",
			Access:      params.ModelAdminAccess,
		},
	}
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
		}, {
			Name:  "model-with-extra-local-users",
			UUID:  "extra-local-users-uuid",
			Type:  model.IAAS,
			Owner: "sourceuser",
		}, {
			Name:  "model-with-extra-external-users",
			UUID:  "extra-external-users-uuid",
			Type:  model.IAAS,
			Owner: "sourceuser",
		}, {
			Name:  "model-with-extra-users",
			UUID:  "extra-users-uuid",
			Type:  model.IAAS,
			Owner: "sourceuser",
		}},
		modelInfo: []params.ModelInfo{
			{
				Name:  "model",
				UUID:  modelUUID,
				Users: userList,
			},
			{
				Name:  "production",
				UUID:  "prod-1-uuid",
				Users: userList,
			},
			{
				Name:  "production",
				UUID:  "prod-2-uuid",
				Users: userList,
			},
			{
				Name: "model-with-extra-local-users",
				UUID: "extra-local-users-uuid",
				Users: []params.ModelUserInfo{
					{
						UserName:    "admin",
						DisplayName: "admin",
						Access:      params.ModelAdminAccess,
					},
					{
						UserName:    "foo",
						DisplayName: "foo",
						Access:      params.ModelReadAccess,
					},
				},
			},
			{
				Name: "model-with-extra-external-users",
				UUID: "extra-external-users-uuid",
				Users: []params.ModelUserInfo{
					{
						UserName:    "admin",
						DisplayName: "admin",
						Access:      params.ModelAdminAccess,
					},
					{
						UserName:    "foo@external",
						DisplayName: "foo",
						Access:      params.ModelReadAccess,
					},
				},
			},
			{
				Name: "model-with-extra-users",
				UUID: "extra-users-uuid",
				Users: []params.ModelUserInfo{
					{
						UserName:    "admin",
						DisplayName: "admin",
						Access:      params.ModelAdminAccess,
					},
					{
						UserName:    "foo@external",
						DisplayName: "foo",
						Access:      params.ModelReadAccess,
					},
					{
						UserName:    "bar",
						DisplayName: "bar",
						Access:      params.ModelReadAccess,
					},
				},
			},
		},
	}

	s.userAPI = &fakeUserAPI{
		users: []params.UserInfo{
			{
				Username: "admin",
			},
		},
	}

	mac0, err := macaroon.New([]byte("secret0"), []byte("id0"), "location0", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	mac1, err := macaroon.New([]byte("secret1"), []byte("id1"), "location1", macaroon.LatestVersion)
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
	cookie, err := httpbakery.NewCookie(nil, macaroon.Slice{mac})
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
		ModelUUID:             modelUUID,
		TargetControllerUUID:  targetControllerUUID,
		TargetControllerAlias: "target",
		TargetAddrs:           []string{"1.2.3.4:5"},
		TargetCACert:          "cert",
		TargetUser:            "targetuser",
		TargetPassword:        "secret",
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
		ModelUUID:             modelUUID,
		TargetControllerUUID:  targetControllerUUID,
		TargetControllerAlias: "target",
		TargetAddrs:           []string{"1.2.3.4:5"},
		TargetCACert:          "cert",
		TargetUser:            "targetuser",
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
		ModelUUID:             "prod-2-uuid",
		TargetControllerUUID:  targetControllerUUID,
		TargetControllerAlias: "target",
		TargetAddrs:           []string{"1.2.3.4:5"},
		TargetCACert:          "cert",
		TargetUser:            "targetuser",
		TargetPassword:        "secret",
	})
}

func (s *MigrateSuite) TestUserMissingFromTarget(c *gc.C) {
	specs := []struct {
		descr          string
		srcModel       string
		srcIdentityURL string
		dstIdentityURL string
		expErr         string
	}{
		{
			descr:    "local model grants access to users not present in target",
			srcModel: "model-with-extra-local-users",
			expErr: `cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - foo`,
		},
		{
			// Even though the same external identity provider is used
			// local users still need to be shared so this should fail.
			descr:          "both controllers share the same external identity provider but some of the local model users are not present in target",
			srcModel:       "model-with-extra-users",
			srcIdentityURL: "https://api.jujucharms.com/identity",
			dstIdentityURL: "https://api.jujucharms.com/identity",
			expErr: `cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - bar`,
		},
		{
			// This should work as the local users are shared and
			// the same external identity provider can authenticate
			// the external users.
			descr:          "local model grants access to external users not present in target but both controllers use same external identity provider",
			srcModel:       "model-with-extra-external-users",
			srcIdentityURL: "https://api.jujucharms.com/identity",
			dstIdentityURL: "https://api.jujucharms.com/identity",
		},
		{
			// This should fail even though the local users are shared
			// as different identity providers are used which means
			// we probably won't be able to auth external users
			// anyway.
			descr:          "local model grants access to external users not present in target but both controllers use different external identity provider",
			srcModel:       "model-with-extra-external-users",
			srcIdentityURL: "https://api.jujucharms.com/identity",
			dstIdentityURL: "https://candid.provider/identity",
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you can remove the following users from the current model:
  - foo@external`,
		},
		{
			// This is a more complicated case. The two controllers
			// use different identity providers which means that we
			// probably won't be able to auth external users. Also,
			// some of the local model's users do not exist in the
			// target.
			descr:          "controllers use different external identity providers AND local model users are not present in target",
			srcModel:       "model-with-extra-users",
			srcIdentityURL: "https://api.jujucharms.com/identity",
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you need to remove the following users from the current model:
  - foo@external

and add the following users to the destination controller or remove them from
the current model:
  - bar`,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)

		cmd := s.makeCommand()
		inner := modelcmd.InnerCommand(cmd).(*migrateCommand)
		inner.migAPI["source"].(*fakeMigrateAPI).identityURL = spec.srcIdentityURL
		inner.migAPI["target"].(*fakeMigrateAPI).identityURL = spec.dstIdentityURL
		_, err := cmdtesting.RunCommand(c, cmd, spec.srcModel, "target")

		if spec.expErr == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.Not(gc.IsNil))
			c.Assert(err.Error(), gc.Equals, spec.expErr)
		}
	}
}

func (s *MigrateSuite) TestSpecifyOwner(c *gc.C) {
	ctx, err := s.makeAndRun(c, "alpha/production", "target")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Migration started with ID \"uuid:0\"\n")
	c.Check(s.api.specSeen.ModelUUID, gc.Equals, "prod-1-uuid")
}

func (s *MigrateSuite) TestControllerDoesNotExist(c *gc.C) {
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
	apiCopy := *s.api
	inner.migAPI = map[string]migrateAPI{
		"source": s.api,
		"target": &apiCopy,
	}
	inner.modelAPI = s.modelAPI
	inner.userAPI = s.userAPI
	inner.newAPIRoot = func(jujuclient.ClientStore, string, string) (api.Connection, error) {
		return s.targetControllerAPI, nil
	}
	return cmd
}

type fakeMigrateAPI struct {
	specSeen    *controller.MigrationSpec
	identityURL string
}

func (a *fakeMigrateAPI) InitiateMigration(spec controller.MigrationSpec) (string, error) {
	a.specSeen = &spec
	return "uuid:0", nil
}

func (a *fakeMigrateAPI) IdentityProviderURL() (string, error) {
	return a.identityURL, nil
}

func (*fakeMigrateAPI) Close() error {
	return nil
}

type fakeModelAPI struct {
	models    []base.UserModel
	modelInfo []params.ModelInfo
}

func (m *fakeModelAPI) ListModels(user string) ([]base.UserModel, error) {
	return m.models, nil
}

func (m *fakeModelAPI) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	var (
		mi  *params.ModelInfo
		err *params.Error
	)

	modelUUID := tags[0].Id()
	for _, model := range m.modelInfo {
		if model.UUID == modelUUID {
			mi = &model
			break
		}
	}

	if mi == nil {
		err = &params.Error{
			Code: params.CodeNotFound,
		}
	}

	return []params.ModelInfoResult{
		{
			Result: mi,
			Error:  err,
		},
	}, nil
}

func (m *fakeModelAPI) Close() error {
	return nil
}

type fakeUserAPI struct {
	users []params.UserInfo
}

func (*fakeUserAPI) Close() error {
	return nil
}

func (a *fakeUserAPI) UserInfo(_ []string, _ usermanager.IncludeDisabled) ([]params.UserInfo, error) {
	return a.users, nil
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
