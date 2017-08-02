// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type modelmanagerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&modelmanagerSuite{})

func (s *modelmanagerSuite) TestCreateModelBadUser(c *gc.C) {
	client := modelmanager.NewClient(basetesting.BestVersionCaller{})
	_, err := client.CreateModel("mymodel", "not a user", "", "", names.CloudCredentialTag{}, nil)
	c.Assert(err, gc.ErrorMatches, `invalid owner name "not a user"`)
}

func (s *modelmanagerSuite) TestCreateModelBadCloud(c *gc.C) {
	client := modelmanager.NewClient(basetesting.BestVersionCaller{})
	_, err := client.CreateModel("mymodel", "bob", "123!", "", names.CloudCredentialTag{}, nil)
	c.Assert(err, gc.ErrorMatches, `invalid cloud name "123!"`)
}

func (s *modelmanagerSuite) TestCreateModel(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ModelManager")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CreateModel")
		c.Check(arg, jc.DeepEquals, params.ModelCreateArgs{
			Name:        "new-model",
			OwnerTag:    "user-bob",
			Config:      map[string]interface{}{"abc": 123},
			CloudTag:    "cloud-nimbus",
			CloudRegion: "catbus",
		})
		c.Check(result, gc.FitsTypeOf, &params.ModelInfo{})

		out := result.(*params.ModelInfo)
		out.Name = "dowhatimean"
		out.UUID = "youyoueyedee"
		out.ControllerUUID = "youyoueyedeetoo"
		out.ProviderType = "C-123"
		out.DefaultSeries = "M*A*S*H"
		out.CloudTag = "cloud-nimbus"
		out.CloudRegion = "catbus"
		out.OwnerTag = "user-fnord"
		out.Life = "alive"
		return nil
	})

	client := modelmanager.NewClient(apiCaller)
	newModel, err := client.CreateModel(
		"new-model",
		"bob",
		"nimbus",
		"catbus",
		names.CloudCredentialTag{},
		map[string]interface{}{"abc": 123},
	)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newModel, jc.DeepEquals, base.ModelInfo{
		Name:           "dowhatimean",
		UUID:           "youyoueyedee",
		ControllerUUID: "youyoueyedeetoo",
		ProviderType:   "C-123",
		DefaultSeries:  "M*A*S*H",
		Cloud:          "nimbus",
		CloudRegion:    "catbus",
		Owner:          "fnord",
		Life:           "alive",
		Status: base.Status{
			Data: make(map[string]interface{}),
		},
		Users:    []base.UserInfo{},
		Machines: []base.Machine{},
	})
}

func (s *modelmanagerSuite) TestListModelsBadUser(c *gc.C) {
	client := modelmanager.NewClient(basetesting.BestVersionCaller{})
	_, err := client.ListModels("not a user")
	c.Assert(err, gc.ErrorMatches, `invalid user name "not a user"`)
}

func (s *modelmanagerSuite) TestListModels(c *gc.C) {
	lastConnection := time.Now()
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, req string,
			args, resp interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(id, gc.Equals, "")
			c.Check(req, gc.Equals, "ListModels")
			c.Check(args, jc.DeepEquals, params.Entity{"user-user@remote"})
			results := resp.(*params.UserModelList)
			results.UserModels = []params.UserModel{{
				Model: params.Model{
					Name:     "yo",
					UUID:     "wei",
					OwnerTag: "user-user@remote",
				},
				LastConnection: &lastConnection,
			}, {
				Model: params.Model{
					Name:     "sup",
					UUID:     "hazzagarn",
					OwnerTag: "user-phyllis@thrace",
				},
			}}
			return nil
		},
	)

	client := modelmanager.NewClient(apiCaller)
	models, err := client.ListModels("user@remote")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, []base.UserModel{{
		Name:           "yo",
		UUID:           "wei",
		Owner:          "user@remote",
		LastConnection: &lastConnection,
	}, {
		Name:  "sup",
		UUID:  "hazzagarn",
		Owner: "phyllis@thrace",
	}})
}

func (s *modelmanagerSuite) TestDestroyModel(c *gc.C) {
	true_ := true
	false_ := false
	s.testDestroyModel(c, nil)
	s.testDestroyModel(c, &true_)
	s.testDestroyModel(c, &false_)
}

func (s *modelmanagerSuite) testDestroyModel(c *gc.C, destroyStorage *bool) {
	var called bool
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 4,
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string,
				version int,
				id, req string,
				args, resp interface{},
			) error {
				c.Check(objType, gc.Equals, "ModelManager")
				c.Check(id, gc.Equals, "")
				c.Check(req, gc.Equals, "DestroyModels")
				c.Check(args, jc.DeepEquals, params.DestroyModelsParams{
					Models: []params.DestroyModelParams{{
						ModelTag:       coretesting.ModelTag.String(),
						DestroyStorage: destroyStorage,
					}},
				})
				results := resp.(*params.ErrorResults)
				*results = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}
				called = true
				return nil
			},
		),
	}
	client := modelmanager.NewClient(apiCaller)
	err := client.DestroyModel(coretesting.ModelTag, destroyStorage)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelmanagerSuite) TestDestroyModelV3(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, req string,
			args, resp interface{},
		) error {
			c.Check(objType, gc.Equals, "ModelManager")
			c.Check(id, gc.Equals, "")
			c.Check(req, gc.Equals, "DestroyModels")
			c.Check(args, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{{coretesting.ModelTag.String()}},
			})
			results := resp.(*params.ErrorResults)
			*results = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			called = true
			return nil
		},
	)
	client := modelmanager.NewClient(apiCaller)
	destroyStorage := true
	err := client.DestroyModel(coretesting.ModelTag, &destroyStorage)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *modelmanagerSuite) TestDestroyModelV3DestroyStorageNotTrue(c *gc.C) {
	client := modelmanager.NewClient(basetesting.BestVersionCaller{})
	for _, destroyStorage := range []*bool{nil, new(bool)} {
		err := client.DestroyModel(coretesting.ModelTag, destroyStorage)
		c.Assert(err, gc.ErrorMatches, "this Juju controller requires destroyStorage to be true")
	}
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

func (s *modelmanagerSuite) TestModelStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "ModelManager")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ModelStatus")
		c.Check(arg, jc.DeepEquals, params.Entities{
			[]params.Entity{{
				Tag: coretesting.ModelTag.String(),
			}},
		})
		c.Check(result, gc.FitsTypeOf, &params.ModelStatusResults{})

		out := result.(*params.ModelStatusResults)
		out.Results = []params.ModelStatus{{
			ModelTag:           coretesting.ModelTag.String(),
			OwnerTag:           "user-glenda",
			ApplicationCount:   3,
			HostedMachineCount: 2,
			Life:               "alive",
			Machines: []params.ModelMachineInfo{{
				Id:         "0",
				InstanceId: "inst-ance",
				Status:     "pending",
			}},
		}}
		return nil
	})

	client := modelmanager.NewClient(apiCaller)
	results, err := client.ModelStatus(coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []base.ModelStatus{{
		UUID:               coretesting.ModelTag.Id(),
		TotalMachineCount:  1,
		HostedMachineCount: 2,
		ServiceCount:       3,
		Owner:              "glenda",
		Life:               string(params.Alive),
		Machines:           []base.Machine{{Id: "0", InstanceId: "inst-ance", Status: "pending"}},
	}})
}

type dumpModelSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&dumpModelSuite{})

func (s *dumpModelSuite) TestDumpModelV3(c *gc.C) {
	expected := map[string]interface{}{
		"model-uuid": "some-uuid",
		"other-key":  "special",
	}
	results := params.StringResults{Results: []params.StringResult{{
		Result: "model-uuid: some-uuid\nother-key: special\n",
	}}}
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 3,
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, args, result interface{}) error {
				c.Check(objType, gc.Equals, "ModelManager")
				c.Check(request, gc.Equals, "DumpModels")
				c.Check(version, gc.Equals, 3)
				c.Assert(args, gc.DeepEquals, params.DumpModelRequest{
					Entities:   []params.Entity{{coretesting.ModelTag.String()}},
					Simplified: true})
				res, ok := result.(*params.StringResults)
				c.Assert(ok, jc.IsTrue)
				*res = results
				return nil
			}),
	}
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(coretesting.ModelTag, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *dumpModelSuite) TestDumpModelV2(c *gc.C) {
	expected := map[string]interface{}{
		"model-uuid": "some-uuid",
		"other-key":  "special",
	}
	results := params.MapResults{Results: []params.MapResult{{
		Result: expected,
	}}}
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 2,
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, args, result interface{}) error {
				c.Check(objType, gc.Equals, "ModelManager")
				c.Check(request, gc.Equals, "DumpModels")
				c.Check(version, gc.Equals, 2)
				c.Assert(args, gc.DeepEquals, params.Entities{[]params.Entity{{coretesting.ModelTag.String()}}})
				res, ok := result.(*params.MapResults)
				c.Assert(ok, jc.IsTrue)
				*res = results
				return nil
			}),
	}
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(coretesting.ModelTag, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, expected)
}

func (s *dumpModelSuite) TestDumpModelErrorV3(c *gc.C) {
	results := params.StringResults{Results: []params.StringResult{{
		Error: &params.Error{Message: "fake error"},
	}}}
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 3,
		APICallerFunc: basetesting.APICallerFunc(func(objType string, version int, id, request string, args, result interface{}) error {
			res, ok := result.(*params.StringResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		}),
	}
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(coretesting.ModelTag, false)
	c.Assert(err, gc.ErrorMatches, "fake error")
	c.Assert(out, gc.IsNil)
}

func (s *dumpModelSuite) TestDumpModelErrorV2(c *gc.C) {
	results := params.MapResults{Results: []params.MapResult{{
		Error: &params.Error{Message: "fake error"},
	}}}
	apiCaller := basetesting.BestVersionCaller{
		BestVersion: 2,
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, args, result interface{}) error {
				res, ok := result.(*params.MapResults)
				c.Assert(ok, jc.IsTrue)
				*res = results
				return nil
			}),
	}
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModel(coretesting.ModelTag, false)
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
			c.Assert(in, gc.DeepEquals, params.Entities{[]params.Entity{{coretesting.ModelTag.String()}}})
			res, ok := result.(*params.MapResults)
			c.Assert(ok, jc.IsTrue)
			*res = results
			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	out, err := client.DumpModelDB(coretesting.ModelTag)
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
	out, err := client.DumpModelDB(coretesting.ModelTag)
	c.Assert(err, gc.ErrorMatches, "fake error")
	c.Assert(out, gc.IsNil)
}
