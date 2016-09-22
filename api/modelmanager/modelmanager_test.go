// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type modelmanagerSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&modelmanagerSuite{})

func (s *modelmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
}

func (s *modelmanagerSuite) OpenAPI(c *gc.C) *modelmanager.Client {
	return modelmanager.NewClient(s.OpenControllerAPI(c))
}

func (s *modelmanagerSuite) TestCreateModelBadUser(c *gc.C) {
	modelManager := s.OpenAPI(c)
	defer modelManager.Close()
	_, err := modelManager.CreateModel("mymodel", "not a user", "", "", names.CloudCredentialTag{}, nil)
	c.Assert(err, gc.ErrorMatches, `invalid owner name "not a user"`)
}

func (s *modelmanagerSuite) TestCreateModel(c *gc.C) {
	s.testCreateModel(c, "dummy", "dummy-region")
}

func (s *modelmanagerSuite) TestCreateModelCloudDefaultRegion(c *gc.C) {
	s.testCreateModel(c, "dummy", "")
}

func (s *modelmanagerSuite) TestCreateModelDefaultCloudAndRegion(c *gc.C) {
	s.testCreateModel(c, "", "")
}

func (s *modelmanagerSuite) testCreateModel(c *gc.C, cloud, region string) {
	modelManager := s.OpenAPI(c)
	defer modelManager.Close()
	user := s.Factory.MakeUser(c, nil)
	owner := user.UserTag().Canonical()
	newModel, err := modelManager.CreateModel("new-model", owner, cloud, region, names.CloudCredentialTag{}, map[string]interface{}{
		"authorized-keys": "ssh-key",
		// dummy needs controller
		"controller": false,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newModel.Name, gc.Equals, "new-model")
	c.Assert(newModel.Owner, gc.Equals, user.String())
	c.Assert(newModel.CloudRegion, gc.Equals, "dummy-region")
	c.Assert(utils.IsValidUUIDString(newModel.UUID), jc.IsTrue)
}

func (s *modelmanagerSuite) TestListModelsBadUser(c *gc.C) {
	modelManager := s.OpenAPI(c)
	defer modelManager.Close()
	_, err := modelManager.ListModels("not a user")
	c.Assert(err, gc.ErrorMatches, `invalid user name "not a user"`)
}

func (s *modelmanagerSuite) TestListModels(c *gc.C) {
	owner := names.NewUserTag("user@remote")
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: owner}).Close()
	s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "second", Owner: owner}).Close()

	modelManager := s.OpenAPI(c)
	defer modelManager.Close()
	models, err := modelManager.ListModels("user@remote")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 2)

	modelNames := []string{models[0].Name, models[1].Name}
	c.Assert(modelNames, jc.DeepEquals, []string{"first", "second"})
	ownerNames := []string{models[0].Owner, models[1].Owner}
	c.Assert(ownerNames, jc.DeepEquals, []string{"user@remote", "user@remote"})
}

func (s *modelmanagerSuite) TestDestroyModel(c *gc.C) {
	modelManager := s.OpenAPI(c)
	defer modelManager.Close()
	var called bool
	modelmanager.PatchFacadeCall(&s.CleanupSuite, modelManager,
		func(req string, args interface{}, resp interface{}) error {
			c.Assert(req, gc.Equals, "DestroyModels")
			c.Assert(args, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{testing.ModelTag.String()}},
			})
			results := resp.(*params.ErrorResults)
			*results = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		})

	err := modelManager.DestroyModel(testing.ModelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelmanagerSuite) TestModelDefaults(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelDefaults")
			c.Check(a, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ModelDefaultsResult{})
			results := result.(*params.ModelDefaultsResult)
			results.Config = map[string]params.ModelDefaults{
				"foo": {"bar", "model", []params.RegionDefaults{{
					"dummy-region",
					"dummy-value"}}},
			}
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	result, err := client.ModelDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, jc.DeepEquals, config.ModelDefaultAttributes{
		"foo": {"bar", "model", []config.RegionDefaultValue{{
			"dummy-region",
			"dummy-value"}}},
	})
}

func (s *modelmanagerSuite) TestSetModelDefaults(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "SetModelDefaults")
			c.Check(a, jc.DeepEquals, params.SetModelDefaults{
				Config: []params.ModelDefaultValues{{
					CloudTag:    "cloud-mycloud",
					CloudRegion: "region",
					Config: map[string]interface{}{
						"some-name":  "value",
						"other-name": true,
					},
				}}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: nil}},
			}
			called = true
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	err := client.SetModelDefaults("mycloud", "region", map[string]interface{}{
		"some-name":  "value",
		"other-name": true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelmanagerSuite) TestUnsetModelDefaults(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UnsetModelDefaults")
			c.Check(a, jc.DeepEquals, params.UnsetModelDefaults{
				Keys: []params.ModelUnsetKeys{{
					CloudTag:    "cloud-mycloud",
					CloudRegion: "region",
					Keys:        []string{"foo", "bar"},
				}}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{Error: nil}},
			}
			called = true
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	err := client.UnsetModelDefaults("mycloud", "region", "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type dumpModelSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&dumpModelSuite{})

func (s *dumpModelSuite) TestDumpModel(c *gc.C) {
	expected := map[string]interface{}{
		"model-uuid": "some-uuid",
		"other-key":  "special",
	}
	results := params.MapResults{Results: []params.MapResult{{
		Result: expected,
	}}}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, result interface{}) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(request, gc.Equals, "DumpModels")
			in, ok := args.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(in, gc.DeepEquals, params.Entities{[]params.Entity{{testing.ModelTag.String()}}})
			res, ok := result.(*params.MapResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(testing.ModelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *dumpModelSuite) TestDumpModelError(c *gc.C) {
	results := params.MapResults{Results: []params.MapResult{{
		Error: &params.Error{Message: "fake error"},
	}}}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, result interface{}) error {
			res, ok := result.(*params.MapResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(testing.ModelTag)
	c.Assert(err, gc.ErrorMatches, "fake error")
	c.Assert(out, gc.IsNil)
}

func (s *dumpModelSuite) TestDumpModelDB(c *gc.C) {
	expected := map[string]interface{}{
		"models": []map[string]interface{}{{
			"name": "admin",
			"uuid": "some-uuid",
		}},
		"machines": []map[string]interface{}{{
			"id":   "0",
			"life": 0,
		}},
	}
	results := params.MapResults{Results: []params.MapResult{{
		Result: expected,
	}}}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, result interface{}) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(request, gc.Equals, "DumpModelsDB")
			in, ok := args.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(in, gc.DeepEquals, params.Entities{[]params.Entity{{testing.ModelTag.String()}}})
			res, ok := result.(*params.MapResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModelDB(testing.ModelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *dumpModelSuite) TestDumpModelDBError(c *gc.C) {
	results := params.MapResults{Results: []params.MapResult{{
		Error: &params.Error{Message: "fake error"},
	}}}
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, args, result interface{}) error {
			res, ok := result.(*params.MapResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModelDB(testing.ModelTag)
	c.Assert(err, gc.ErrorMatches, "fake error")
	c.Assert(out, gc.IsNil)
}
