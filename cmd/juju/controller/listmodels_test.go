// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
)

type ModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeModelMgrAPIClient
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ModelsSuite{})

type fakeModelMgrAPIClient struct {
	err    error
	user   string
	models []base.UserModel
	all    bool
}

func (f *fakeModelMgrAPIClient) Close() error {
	return nil
}

func (f *fakeModelMgrAPIClient) ListModels(user string) ([]base.UserModel, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.user = user
	return f.models, nil
}

func (f *fakeModelMgrAPIClient) AllModels() ([]base.UserModel, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.all = true
	return f.models, nil
}

func (f *fakeModelMgrAPIClient) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	results := make([]params.ModelInfoResult, len(tags))
	for i, tag := range tags {
		for _, model := range f.models {
			if model.UUID != tag.Id() {
				continue
			}
			result := &params.ModelInfo{
				Name:     model.Name,
				UUID:     model.UUID,
				OwnerTag: names.NewUserTag(model.Owner).String(),
			}
			switch model.Name {
			case "test-model1":
				last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
				result.Status.Status = status.StatusActive
				if f.user != "" {
					result.Users = []params.ModelUserInfo{{
						UserName:       f.user,
						LastConnection: &last1,
					}}
				}
			case "test-model2":
				last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)
				result.Status.Status = status.StatusActive
				if f.user != "" {
					result.Users = []params.ModelUserInfo{{
						UserName:       f.user,
						LastConnection: &last2,
					}}
				}
			case "test-model3":
				result.Status.Status = status.StatusDestroying
			}
			results[i].Result = result
		}
	}
	return results, nil
}

func (s *ModelsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	models := []base.UserModel{
		{
			Name:  "test-model1",
			Owner: "user-admin@local",
			UUID:  "test-model1-UUID",
		}, {
			Name:  "test-model2",
			Owner: "user-admin@local",
			UUID:  "test-model2-UUID",
		}, {
			Name:  "test-model3",
			Owner: "user-admin@local",
			UUID:  "test-model3-UUID",
		},
	}
	s.api = &fakeModelMgrAPIClient{
		models: models,
		user:   "admin@local",
	}
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "fake"
	s.store.Controllers["fake"] = jujuclient.ControllerDetails{}
	s.store.Models["fake"] = jujuclient.ControllerAccountModels{
		AccountModels: map[string]*jujuclient.AccountModels{
			"admin@local": {
				CurrentModel: "test-model1",
			},
		},
	}
	s.store.Accounts["fake"] = &jujuclient.ControllerAccounts{
		Accounts: map[string]jujuclient.AccountDetails{
			"admin@local": {
				User:     "admin@local",
				Password: "password",
			},
		},
		CurrentAccount: "admin@local",
	}
}

func (s *ModelsSuite) newCommand() cmd.Command {
	return controller.NewListModelsCommandForTest(s.api, s.api, s.store)
}

func (s *ModelsSuite) checkSuccess(c *gc.C, user string, args ...string) {
	context, err := testing.RunCommand(c, s.newCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, user)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"MODEL         OWNER             STATUS      LAST CONNECTION\n"+
		"test-model1*  user-admin@local  active      2015-03-20\n"+
		"test-model2   user-admin@local  active      2015-03-01\n"+
		"test-model3   user-admin@local  destroying  never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestModels(c *gc.C) {
	s.checkSuccess(c, "admin@local")
	s.checkSuccess(c, "bob", "--user", "bob")
}

func (s *ModelsSuite) TestAllModels(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.all, jc.IsTrue)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"MODEL         OWNER             STATUS      LAST CONNECTION\n"+
		"test-model1*  user-admin@local  active      2015-03-20\n"+
		"test-model2   user-admin@local  active      2015-03-01\n"+
		"test-model3   user-admin@local  destroying  never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestAllModelsNoneCurrent(c *gc.C) {
	delete(s.store.Models, "fake")
	context, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"MODEL        OWNER             STATUS      LAST CONNECTION\n"+
		"test-model1  user-admin@local  active      2015-03-20\n"+
		"test-model2  user-admin@local  active      2015-03-01\n"+
		"test-model3  user-admin@local  destroying  never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestModelsUUID(c *gc.C) {
	context, err := testing.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin@local")
	c.Assert(testing.Stdout(context), gc.Equals, ""+
		"MODEL         MODEL UUID        OWNER             STATUS      LAST CONNECTION\n"+
		"test-model1*  test-model1-UUID  user-admin@local  active      2015-03-20\n"+
		"test-model2   test-model2-UUID  user-admin@local  active      2015-03-01\n"+
		"test-model3   test-model3-UUID  user-admin@local  destroying  never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *ModelsSuite) TestModelsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := testing.RunCommand(c, s.newCommand())
	c.Assert(err, gc.ErrorMatches, "cannot list models: permission denied")
}
