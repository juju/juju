// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	coretesting "github.com/juju/juju/testing"
)

type Suite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&Suite{})

func (s *Suite) TestDestroyControllerAPIVersion(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{BestVersion: 3}
	client := controller.NewClient(apiCaller)
	for _, destroyStorage := range []*bool{nil, new(bool)} {
		err := client.DestroyController(controller.DestroyControllerParams{
			DestroyStorage: destroyStorage,
		})
		c.Assert(err, gc.ErrorMatches, "this Juju controller requires DestroyStorage to be true")
	}

}

func (s *Suite) TestDestroyController(c *gc.C) {
	var stub jujutesting.Stub
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 4,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			stub.AddCall(objType+"."+request, arg)
			return stub.NextErr()
		},
	}
	client := controller.NewClient(apiCaller)

	destroyStorage := true
	err := client.DestroyController(controller.DestroyControllerParams{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	c.Assert(err, jc.ErrorIsNil)

	stub.CheckCalls(c, []jujutesting.StubCall{
		{"Controller.DestroyController", []interface{}{params.DestroyControllerArgs{
			DestroyModels:  true,
			DestroyStorage: &destroyStorage,
		}}},
	})
}

func (s *Suite) TestDestroyControllerError(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 4,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			return errors.New("nope")
		},
	}
	client := controller.NewClient(apiCaller)
	err := client.DestroyController(controller.DestroyControllerParams{})
	c.Assert(err, gc.ErrorMatches, "nope")
}

func (s *Suite) TestInitiateMigration(c *gc.C) {
	s.checkInitiateMigration(c, makeSpec())
}

func (s *Suite) TestInitiateMigrationEmptyCACert(c *gc.C) {
	spec := makeSpec()
	spec.TargetCACert = ""
	s.checkInitiateMigration(c, spec)
}

func (s *Suite) checkInitiateMigration(c *gc.C, spec controller.MigrationSpec) {
	client, stub := makeInitiateMigrationClient(params.InitiateMigrationResults{
		Results: []params.InitiateMigrationResult{{
			MigrationId: "id",
		}},
	})
	id, err := client.InitiateMigration(spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, gc.Equals, "id")
	stub.CheckCalls(c, []jujutesting.StubCall{
		{"Controller.InitiateMigration", []interface{}{specToArgs(spec)}},
	})
}

func specToArgs(spec controller.MigrationSpec) params.InitiateMigrationArgs {
	var macsJSON []byte
	if len(spec.TargetMacaroons) > 0 {
		var err error
		macsJSON, err = json.Marshal(spec.TargetMacaroons)
		if err != nil {
			panic(err)
		}
	}
	return params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: names.NewModelTag(spec.ModelUUID).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(spec.TargetControllerUUID).String(),
				Addrs:         spec.TargetAddrs,
				CACert:        spec.TargetCACert,
				AuthTag:       names.NewUserTag(spec.TargetUser).String(),
				Password:      spec.TargetPassword,
				Macaroons:     string(macsJSON),
			},
		}},
	}
}

func (s *Suite) TestInitiateMigrationError(c *gc.C) {
	client, _ := makeInitiateMigrationClient(params.InitiateMigrationResults{
		Results: []params.InitiateMigrationResult{{
			Error: common.ServerError(errors.New("boom")),
		}},
	})
	id, err := client.InitiateMigration(makeSpec())
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestInitiateMigrationResultMismatch(c *gc.C) {
	client, _ := makeInitiateMigrationClient(params.InitiateMigrationResults{
		Results: []params.InitiateMigrationResult{
			{MigrationId: "id"},
			{MigrationId: "wtf"},
		},
	})
	id, err := client.InitiateMigration(makeSpec())
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "unexpected number of results returned")
}

func (s *Suite) TestInitiateMigrationCallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := controller.NewClient(apiCaller)
	id, err := client.InitiateMigration(makeSpec())
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestInitiateMigrationValidationError(c *gc.C) {
	client, stub := makeInitiateMigrationClient(params.InitiateMigrationResults{})
	spec := makeSpec()
	spec.ModelUUID = "not-a-uuid"
	id, err := client.InitiateMigration(spec)
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "client-side validation failed: model UUID not valid")
	c.Check(stub.Calls(), gc.HasLen, 0) // API call shouldn't have happened
}

func (s *Suite) TestHostedModelConfigs_CallError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(string, int, string, string, interface{}, interface{}) error {
		return errors.New("boom")
	})
	client := controller.NewClient(apiCaller)
	config, err := client.HostedModelConfigs()
	c.Check(config, gc.HasLen, 0)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestHostedModelConfigs_FormatResults(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Controller")
		c.Assert(request, gc.Equals, "HostedModelConfigs")
		c.Assert(arg, gc.IsNil)
		out := result.(*params.HostedModelConfigsResults)
		c.Assert(out, gc.NotNil)
		*out = params.HostedModelConfigsResults{
			Models: []params.HostedModelConfig{
				{
					Name:     "first",
					OwnerTag: "user-foo@bar",
					Config: map[string]interface{}{
						"name": "first",
					},
					CloudSpec: &params.CloudSpec{
						Type: "magic",
						Name: "first",
					},
				}, {
					Name:     "second",
					OwnerTag: "bad-tag",
				}, {
					Name:     "third",
					OwnerTag: "user-foo@bar",
					Config: map[string]interface{}{
						"name": "third",
					},
					CloudSpec: &params.CloudSpec{
						Name: "third",
					},
				},
			},
		}
		return nil
	})
	client := controller.NewClient(apiCaller)
	config, err := client.HostedModelConfigs()
	c.Assert(config, gc.HasLen, 3)
	c.Assert(err, jc.ErrorIsNil)
	first := config[0]
	c.Assert(first.Name, gc.Equals, "first")
	c.Assert(first.Owner, gc.Equals, names.NewUserTag("foo@bar"))
	c.Assert(first.Config, gc.DeepEquals, map[string]interface{}{
		"name": "first",
	})
	c.Assert(first.CloudSpec, gc.DeepEquals, environs.CloudSpec{
		Type: "magic",
		Name: "first",
	})
	second := config[1]
	c.Assert(second.Name, gc.Equals, "second")
	c.Assert(second.Error.Error(), gc.Equals, `"bad-tag" is not a valid tag`)
	third := config[2]
	c.Assert(third.Name, gc.Equals, "third")
	c.Assert(third.Error.Error(), gc.Equals, "validating CloudSpec: empty Type not valid")
}

func makeInitiateMigrationClient(results params.InitiateMigrationResults) (
	*controller.Client, *jujutesting.Stub,
) {
	var stub jujutesting.Stub
	apiCaller := apitesting.APICallerFunc(
		func(objType string, version int, id, request string, arg, result interface{}) error {
			stub.AddCall(objType+"."+request, arg)
			out := result.(*params.InitiateMigrationResults)
			*out = results
			return nil
		},
	)
	client := controller.NewClient(apiCaller)
	return client, &stub
}

func makeSpec() controller.MigrationSpec {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location")
	if err != nil {
		panic(err)
	}
	return controller.MigrationSpec{
		ModelUUID:            randomUUID(),
		TargetControllerUUID: randomUUID(),
		TargetAddrs:          []string{"1.2.3.4:5"},
		TargetCACert:         "cert",
		TargetUser:           "someone",
		TargetPassword:       "secret",
		TargetMacaroons:      []macaroon.Slice{{mac}},
	}
}

func randomUUID() string {
	return utils.MustNewUUID().String()
}

func (s *Suite) TestModelStatusEmpty(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Controller")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ModelStatus")
		c.Check(result, gc.FitsTypeOf, &params.ModelStatusResults{})

		return nil
	})

	client := controller.NewClient(apiCaller)
	results, err := client.ModelStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []base.ModelStatus{})
}

func (s *Suite) TestModelStatus(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 4,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "Controller")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ModelStatus")
			c.Check(arg, jc.DeepEquals, params.Entities{
				[]params.Entity{
					{Tag: coretesting.ModelTag.String()},
					{Tag: coretesting.ModelTag.String()},
				},
			})
			c.Check(result, gc.FitsTypeOf, &params.ModelStatusResults{})

			out := result.(*params.ModelStatusResults)
			out.Results = []params.ModelStatus{
				params.ModelStatus{
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
				},
				params.ModelStatus{Error: common.ServerError(errors.New("model error"))},
			}
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	results, err := client.ModelStatus(coretesting.ModelTag, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results[0], jc.DeepEquals, base.ModelStatus{
		UUID:               coretesting.ModelTag.Id(),
		TotalMachineCount:  1,
		HostedMachineCount: 2,
		ApplicationCount:   3,
		Owner:              "glenda",
		Life:               string(params.Alive),
		Machines:           []base.Machine{{Id: "0", InstanceId: "inst-ance", Status: "pending"}},
	})
	c.Assert(results[1].Error, gc.ErrorMatches, "model error")
}

func (s *Suite) TestModelStatusError(c *gc.C) {
	apiCaller := apitesting.APICallerFunc(
		func(objType string, version int, id, request string, args, result interface{}) error {
			return errors.New("model error")
		})
	client := controller.NewClient(apiCaller)
	out, err := client.ModelStatus(coretesting.ModelTag, coretesting.ModelTag)
	c.Assert(err, gc.ErrorMatches, "model error")
	c.Assert(out, gc.IsNil)
}

func (s *Suite) TestConfigSet(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 5,
		APICallerFunc: func(objType string, version int, id, request string, args, result interface{}) error {
			c.Assert(objType, gc.Equals, "Controller")
			c.Assert(version, gc.Equals, 5)
			c.Assert(request, gc.Equals, "ConfigSet")
			c.Assert(result, gc.IsNil)
			c.Assert(args, gc.DeepEquals, params.ControllerConfigSet{Config: map[string]interface{}{
				"some-setting": 345,
			}})
			return errors.New("ruth mundy")
		},
	}
	client := controller.NewClient(apiCaller)
	err := client.ConfigSet(map[string]interface{}{
		"some-setting": 345,
	})
	c.Assert(err, gc.ErrorMatches, "ruth mundy")
}

func (s *Suite) TestConfigSetAgainstOlderAPIVersion(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{BestVersion: 4}
	client := controller.NewClient(apiCaller)
	err := client.ConfigSet(map[string]interface{}{
		"some-setting": 345,
	})
	c.Assert(err, gc.ErrorMatches, "this controller version doesn't support updating controller config")
}
