// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
)

type ShowCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake           fakeModelShowClient
	store          *jujuclienttesting.MemStore
	expectedOutput attrs
}

var _ = gc.Suite(&ShowCommandSuite{})

type fakeModelShowClient struct {
	gitjujutesting.Stub
	info params.ModelInfo
	err  *params.Error
}

func (f *fakeModelShowClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelShowClient) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	f.MethodCall(f, "ModelInfo", tags)
	if len(tags) != 1 {
		return nil, errors.Errorf("expected 1 tag, got %d", len(tags))
	}
	if tags[0] != testing.ModelTag {
		return nil, errors.Errorf("expected %s, got %s", testing.ModelTag.Id(), tags[0].Id())
	}
	return []params.ModelInfoResult{{Result: &f.info, Error: f.err}}, f.NextErr()
}

type attrs map[string]interface{}

func (s *ShowCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	lastConnection := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	statusSince := time.Date(2016, 4, 5, 0, 0, 0, 0, time.UTC)
	migrationStart := time.Date(2016, 4, 6, 0, 10, 0, 0, time.UTC)
	migrationEnd := time.Date(2016, 4, 7, 0, 0, 15, 0, time.UTC)

	users := []params.ModelUserInfo{{
		UserName:       "admin",
		LastConnection: &lastConnection,
		Access:         "write",
	}, {
		UserName:    "bob",
		DisplayName: "Bob",
		Access:      "read",
	}}

	s.fake.ResetCalls()
	s.fake.err = nil
	s.fake.info = params.ModelInfo{
		Name:           "mymodel",
		UUID:           testing.ModelTag.Id(),
		ControllerUUID: "1ca2293b-fdb9-4299-97d6-55583bb39364",
		OwnerTag:       "user-admin",
		CloudTag:       "cloud-some-cloud",
		CloudRegion:    "some-region",
		ProviderType:   "openstack",
		Life:           params.Alive,
		Status: params.EntityStatus{
			Status: status.Active,
			Since:  &statusSince,
		},
		Users: users,
		Migration: &params.ModelMigrationStatus{
			Status: "obfuscating Quigley matrix",
			Start:  &migrationStart,
			End:    &migrationEnd,
		},
	}

	s.expectedOutput = attrs{
		"mymodel": attrs{
			"name":            "mymodel",
			"model-uuid":      "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			"controller-uuid": "1ca2293b-fdb9-4299-97d6-55583bb39364",
			"controller-name": "testing",
			"owner":           "admin",
			"cloud":           "some-cloud",
			"region":          "some-region",
			"type":            "openstack",
			"life":            "alive",
			"status": attrs{
				"current":         "active",
				"since":           "2016-04-05",
				"migration":       "obfuscating Quigley matrix",
				"migration-start": "2016-04-06",
				"migration-end":   "2016-04-07",
			},
			"users": attrs{
				"admin": attrs{
					"access":          "write",
					"last-connection": "2015-03-20",
				},
				"bob": attrs{
					"display-name":    "Bob",
					"access":          "read",
					"last-connection": "never connected",
				},
			},
		},
	}

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"
}

func (s *ShowCommandSuite) newShowCommand() cmd.Command {
	return model.NewShowCommandForTest(&s.fake, noOpRefresh, s.store)
}

func (s *ShowCommandSuite) TestShow(c *gc.C) {
	_, err := testing.RunCommand(c, s.newShowCommand())
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []gitjujutesting.StubCall{
		{"ModelInfo", []interface{}{[]names.ModelTag{testing.ModelTag}}},
		{"Close", nil},
	})
}

func (s *ShowCommandSuite) TestShowUnknownCallsRefresh(c *gc.C) {
	called := false
	refresh := func(jujuclient.ClientStore, string) error {
		called = true
		return nil
	}
	_, err := testing.RunCommand(c, model.NewShowCommandForTest(&s.fake, refresh, s.store), "unknown")
	c.Check(called, jc.IsTrue)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ShowCommandSuite) TestShowFormatYaml(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.newShowCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.YAMLEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestShowFormatJson(c *gc.C) {
	ctx, err := testing.RunCommand(c, s.newShowCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), jc.JSONEquals, s.expectedOutput)
}

func (s *ShowCommandSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, s.newShowCommand(), "admin", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func noOpRefresh(jujuclient.ClientStore, string) error {
	return nil
}
