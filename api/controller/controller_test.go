// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"encoding/json"
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/testing"
	"github.com/juju/utils"
)

type Suite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&Suite{})

func (s *Suite) TestInitiateMigration(c *gc.C) {
	s.checkInitiateMigration(c, makeSpec())
}

func (s *Suite) TestInitiateMigrationExternalControl(c *gc.C) {
	spec := makeSpec()
	spec.ExternalControl = true
	s.checkInitiateMigration(c, spec)
}

func (s *Suite) TestInitiateMigrationSkipPrechecks(c *gc.C) {
	spec := makeSpec()
	spec.SkipInitialPrechecks = true
	s.checkInitiateMigration(c, spec)
}

func (s *Suite) checkInitiateMigration(c *gc.C, spec controller.MigrationSpec) {
	client, stub := makeClient(params.InitiateMigrationResults{
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
			ExternalControl:      spec.ExternalControl,
			SkipInitialPrechecks: spec.SkipInitialPrechecks,
		}},
	}
}

func (s *Suite) TestInitiateMigrationError(c *gc.C) {
	client, _ := makeClient(params.InitiateMigrationResults{
		Results: []params.InitiateMigrationResult{{
			Error: common.ServerError(errors.New("boom")),
		}},
	})
	id, err := client.InitiateMigration(makeSpec())
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestInitiateMigrationResultMismatch(c *gc.C) {
	client, _ := makeClient(params.InitiateMigrationResults{
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
	client, stub := makeClient(params.InitiateMigrationResults{})
	spec := makeSpec()
	spec.ModelUUID = "not-a-uuid"
	id, err := client.InitiateMigration(spec)
	c.Check(id, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "model UUID not valid")
	c.Check(stub.Calls(), gc.HasLen, 0) // API call shouldn't have happened
}

func makeClient(results params.InitiateMigrationResults) (
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
	mac, err := macaroon.New([]byte("secret"), "id", "location")
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
