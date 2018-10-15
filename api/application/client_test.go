// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/api/application"
	basetesting "github.com/juju/juju/api/base/testing"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

type applicationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&applicationSuite{})

func newClient(f basetesting.APICallerFunc) *application.Client {
	return application.NewClient(basetesting.BestVersionCaller{f, 8})
}

func newClientV4(f basetesting.APICallerFunc) *application.Client {
	return application.NewClient(basetesting.BestVersionCaller{f, 4})
}

func (s *applicationSuite) TestSetApplicationMetricCredentials(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "Application")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetMetricCredentials")
		args, ok := a.(params.ApplicationMetricCredentials)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Creds, gc.HasLen, 1)
		c.Assert(args.Creds[0].ApplicationName, gc.Equals, "applicationA")
		c.Assert(args.Creds[0].MetricCredentials, gc.DeepEquals, []byte("creds 1"))

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := client.SetMetricCredentials("applicationA", []byte("creds 1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestSetApplicationMetricCredentialsFails(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "Application")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetMetricCredentials")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return result.OneError()
	})
	err := client.SetMetricCredentials("application", []byte("creds"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDeploy(c *gc.C) {
	var called bool
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				called = true
				c.Assert(request, gc.Equals, "Deploy")
				args, ok := a.(params.ApplicationsDeploy)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args.Applications, gc.HasLen, 1)
				app := args.Applications[0]
				c.Assert(app.CharmURL, gc.Equals, "cs:trusty/a-charm-1")
				c.Assert(app.ApplicationName, gc.Equals, "applicationA")
				c.Assert(app.Series, gc.Equals, "series")
				c.Assert(app.NumUnits, gc.Equals, 1)
				c.Assert(app.ConfigYAML, gc.Equals, "configYAML")
				c.Assert(app.Config, gc.DeepEquals, map[string]string{"foo": "bar"})
				c.Assert(app.Constraints, gc.DeepEquals, constraints.MustParse("mem=4G"))
				c.Assert(app.Placement, gc.DeepEquals, []*instance.Placement{{"scope", "directive"}})
				c.Assert(app.EndpointBindings, gc.DeepEquals, map[string]string{"foo": "bar"})
				c.Assert(app.Storage, gc.DeepEquals, map[string]storage.Constraints{"data": {Pool: "pool"}})
				c.Assert(app.AttachStorage, gc.DeepEquals, []string{"storage-data-0"})
				c.Assert(app.Resources, gc.DeepEquals, map[string]string{"foo": "bar"})

				result := response.(*params.ErrorResults)
				result.Results = make([]params.ErrorResult, 1)
				return nil
			},
		),
		BestVersion: 5,
	})

	args := application.DeployArgs{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("trusty/a-charm-1"),
		},
		ApplicationName:  "applicationA",
		Series:           "series",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Config:           map[string]string{"foo": "bar"},
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{"scope", "directive"}},
		Storage:          map[string]storage.Constraints{"data": {Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}
	err := client.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDeployAttachStorageV4(c *gc.C) {
	var called bool
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				called = true
				return nil
			},
		),
		BestVersion: 4, // v4 does not support AttachStorage
	})
	args := application.DeployArgs{
		NumUnits:      1,
		AttachStorage: []string{"data/0"},
	}
	err := client.Deploy(args)
	c.Assert(err, gc.ErrorMatches, "this juju controller does not support AttachStorage")
	c.Assert(called, jc.IsFalse)
}

func (s *applicationSuite) TestDeployAttachStorageMultipleUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		return nil
	})
	args := application.DeployArgs{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	}
	err := client.Deploy(args)
	c.Assert(err, gc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
	c.Assert(called, jc.IsFalse)
}

func (s *applicationSuite) TestAddUnits(c *gc.C) {
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "AddUnits")
				args, ok := a.(params.AddApplicationUnits)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args.ApplicationName, gc.Equals, "foo")
				c.Assert(args.NumUnits, gc.Equals, 1)
				c.Assert(args.Placement, jc.DeepEquals, []*instance.Placement{{"scope", "directive"}})
				c.Assert(args.AttachStorage, jc.DeepEquals, []string{"storage-data-0"})
				result := response.(*params.AddApplicationUnitsResults)
				result.Units = []string{"foo/0"}
				return nil
			},
		),
		BestVersion: 5,
	})

	units, err := client.AddUnits(application.AddUnitsParams{
		ApplicationName: "foo",
		NumUnits:        1,
		Placement:       []*instance.Placement{{"scope", "directive"}},
		AttachStorage:   []string{"data/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, jc.DeepEquals, []string{"foo/0"})
}

func (s *applicationSuite) TestAddUnitsAttachStorageV4(c *gc.C) {
	var called bool
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				called = true
				return nil
			},
		),
		BestVersion: 4, // v4 does not support AttachStorage
	})

	_, err := client.AddUnits(application.AddUnitsParams{
		NumUnits:      1,
		AttachStorage: []string{"data/0"},
	})
	c.Assert(err, gc.ErrorMatches, "this juju controller does not support AttachStorage")
	c.Assert(called, jc.IsFalse)
}

func (s *applicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		return nil
	})
	_, err := client.AddUnits(application.AddUnitsParams{
		NumUnits:      2,
		AttachStorage: []string{"data/0"},
	})
	c.Assert(err, gc.ErrorMatches, "cannot attach existing storage when more than one unit is requested")
	c.Assert(called, jc.IsFalse)
}

func (s *applicationSuite) TestApplicationGetCharmURL(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "GetCharmURL")
		args, ok := a.(params.ApplicationGet)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")

		result := response.(*params.StringResult)
		result.Result = "curl"
		return nil
	})
	curl, err := client.GetCharmURL("application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("curl"))
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestSetCharm(c *gc.C) {
	var called bool
	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "SetCharm")
		args, ok := a.(params.ApplicationSetCharm)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")
		c.Assert(args.CharmURL, gc.Equals, "cs:trusty/application-1")
		c.Assert(args.ConfigSettings, jc.DeepEquals, map[string]string{
			"a": "b",
			"c": "d",
		})
		c.Assert(args.ConfigSettingsYAML, gc.Equals, "yaml")
		c.Assert(args.ForceSeries, gc.Equals, true)
		c.Assert(args.ForceUnits, gc.Equals, true)
		c.Assert(args.StorageConstraints, jc.DeepEquals, map[string]params.StorageConstraints{
			"a": {Pool: "radiant"},
			"b": {Count: toUint64Ptr(123)},
			"c": {Size: toUint64Ptr(123)},
		})

		return nil
	})
	cfg := application.SetCharmConfig{
		ApplicationName: "application",
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("trusty/application-1"),
		},
		ConfigSettings: map[string]string{
			"a": "b",
			"c": "d",
		},
		ConfigSettingsYAML: "yaml",
		ForceSeries:        true,
		ForceUnits:         true,
		StorageConstraints: map[string]storage.Constraints{
			"a": {Pool: "radiant"},
			"b": {Count: 123},
			"c": {Size: 123},
		},
	}
	err := client.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyDeprecated(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "Destroy")
		c.Assert(a, jc.DeepEquals, params.ApplicationDestroy{
			ApplicationName: "foo",
		})
		return nil
	})
	err := client.DestroyDeprecated("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyUnitsDeprecated(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "DestroyUnits")
		c.Assert(a, jc.DeepEquals, params.DestroyApplicationUnits{
			UnitNames: []string{"foo/0", "bar/1"},
		})
		return nil
	})
	err := client.DestroyUnitsDeprecated("foo/0", "bar/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyApplications(c *gc.C) {
	expectedResults := []params.DestroyApplicationResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyApplicationInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
			DestroyedUnits:   []params.Entity{{Tag: "unit-bar-1"}},
		},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyApplication")
		c.Assert(a, jc.DeepEquals, params.DestroyApplicationsParams{
			Applications: []params.DestroyApplicationParams{
				{ApplicationTag: "application-foo"},
				{ApplicationTag: "application-bar"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyApplicationResults{})
		out := response.(*params.DestroyApplicationResults)
		*out = params.DestroyApplicationResults{expectedResults}
		return nil
	})
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo", "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyApplicationsV4(c *gc.C) {
	expectedResults := []params.DestroyApplicationResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyApplicationInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
			DestroyedUnits:   []params.Entity{{Tag: "unit-bar-1"}},
		},
	}}
	client := newClientV4(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyApplication")
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{
				{Tag: "application-foo"},
				{Tag: "application-bar"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyApplicationResults{})
		out := response.(*params.DestroyApplicationResults)
		*out = params.DestroyApplicationResults{expectedResults}
		return nil
	})
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo", "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyApplicationsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo"},
	})
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyApplicationsInvalidIds(c *gc.C) {
	expectedResults := []params.DestroyApplicationResult{{
		Error: &params.Error{Message: `application name "!" not valid`},
	}, {
		Info: &params.DestroyApplicationInfo{},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		out := response.(*params.DestroyApplicationResults)
		*out = params.DestroyApplicationResults{expectedResults[1:]}
		return nil
	})
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"!", "foo"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyConsumedApplications(c *gc.C) {
	expectedResults := []params.ErrorResult{{
		Error: &params.Error{Message: "boo"},
	}, {}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyConsumedApplications")
		c.Assert(a, jc.DeepEquals, params.DestroyConsumedApplicationsParams{
			Applications: []params.DestroyConsumedApplicationParams{
				{ApplicationTag: "application-foo"},
				{ApplicationTag: "application-bar"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.ErrorResults{})
		out := response.(*params.ErrorResults)
		*out = params.ErrorResults{expectedResults}
		return nil
	})
	results, err := client.DestroyConsumedApplication("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyConsumedApplication("foo")
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyUnits(c *gc.C) {
	expectedResults := []params.DestroyUnitResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyUnitInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyUnit")
		c.Assert(a, jc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{UnitTag: "unit-foo-0"},
				{UnitTag: "unit-bar-1"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyUnitResults{})
		out := response.(*params.DestroyUnitResults)
		*out = params.DestroyUnitResults{expectedResults}
		return nil
	})
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"foo/0", "bar/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyUnitsV4(c *gc.C) {
	expectedResults := []params.DestroyUnitResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyUnitInfo{
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}
	client := newClientV4(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyUnit")
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{
				{Tag: "unit-foo-0"},
				{Tag: "unit-bar-1"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyUnitResults{})
		out := response.(*params.DestroyUnitResults)
		*out = params.DestroyUnitResults{expectedResults}
		return nil
	})
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"foo/0", "bar/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyUnitsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"foo/0"},
	})
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyUnitsInvalidIds(c *gc.C) {
	expectedResults := []params.DestroyUnitResult{{
		Error: &params.Error{Message: `unit ID "!" not valid`},
	}, {
		Info: &params.DestroyUnitInfo{},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		out := response.(*params.DestroyUnitResults)
		*out = params.DestroyUnitResults{expectedResults[1:]}
		return nil
	})
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"!", "foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestConsume(c *gc.C) {
	offer := params.ApplicationOfferDetails{
		SourceModelTag:         "source model",
		OfferName:              "an offer",
		OfferUUID:              "offer-uuid",
		OfferURL:               "offer url",
		ApplicationDescription: "description",
		Endpoints:              []params.RemoteEndpoint{{Name: "endpoint"}},
	}
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	controllerInfo := &params.ExternalControllerInfo{
		ControllerTag: coretesting.ControllerTag.String(),
		Alias:         "controller-alias",
		Addrs:         []string{"192.168.1.0"},
		CACert:        coretesting.CACert,
	}

	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Assert(request, gc.Equals, "Consume")
			args, ok := a.(params.ConsumeApplicationArgs)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Args, jc.DeepEquals, []params.ConsumeApplicationArg{
				{
					ApplicationAlias:        "alias",
					ApplicationOfferDetails: offer,
					Macaroon:                mac,
					ControllerInfo:          controllerInfo,
				},
			})
			if results, ok := result.(*params.ErrorResults); ok {
				result := params.ErrorResult{}
				results.Results = []params.ErrorResult{result}
			}
			return nil
		})
	client := application.NewClient(apiCaller)
	name, err := client.Consume(crossmodel.ConsumeApplicationArgs{
		Offer:            offer,
		ApplicationAlias: "alias",
		Macaroon:         mac,
		ControllerInfo: &crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
			Alias:         "controller-alias",
			Addrs:         controllerInfo.Addrs,
			CACert:        controllerInfo.CACert,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "alias")
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyRelation(c *gc.C) {
	called := false
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyRelation")
		c.Assert(a, jc.DeepEquals, params.DestroyRelation{
			Endpoints: []string{"ep1", "ep2"},
		})
		c.Assert(response, gc.IsNil)
		called = true
		return nil
	})
	err := client.DestroyRelation("ep1", "ep2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyRelationId(c *gc.C) {
	called := false
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyRelation")
		c.Assert(a, jc.DeepEquals, params.DestroyRelation{
			RelationId: 123,
		})
		c.Assert(response, gc.IsNil)
		called = true
		return nil
	})
	err := client.DestroyRelationId(123)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestSetRelationSuspended(c *gc.C) {
	called := false
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Assert(request, gc.Equals, "SetRelationsSuspended")
		c.Assert(a, jc.DeepEquals, params.RelationSuspendedArgs{
			Args: []params.RelationSuspendedArg{
				{
					RelationId: 123,
					Suspended:  true,
					Message:    "message",
				}, {
					RelationId: 456,
					Suspended:  true,
					Message:    "message",
				}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*result.(*params.ErrorResults) = params.ErrorResults{
			Results: []params.ErrorResult{{}, {}},
		}
		called = true
		return nil
	})
	err := client.SetRelationSuspended([]int{123, 456}, true, "message")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestSetRelationSuspendedArity(c *gc.C) {
	called := false
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Assert(request, gc.Equals, "SetRelationsSuspended")
		c.Assert(a, jc.DeepEquals, params.RelationSuspendedArgs{
			Args: []params.RelationSuspendedArg{
				{
					RelationId: 123,
					Suspended:  true,
					Message:    "message",
				}, {
					RelationId: 456,
					Suspended:  true,
					Message:    "message",
				}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*result.(*params.ErrorResults) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		called = true
		return nil
	})
	err := client.SetRelationSuspended([]int{123, 456}, true, "message")
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 1")
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestAddRelation(c *gc.C) {
	called := false
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Assert(request, gc.Equals, "AddRelation")
		c.Assert(a, jc.DeepEquals, params.AddRelation{
			Endpoints: []string{"ep1", "ep2"},
			ViaCIDRs:  []string{"cidr1", "cidr2"},
		})
		c.Assert(result, gc.FitsTypeOf, &params.AddRelationResults{})
		*result.(*params.AddRelationResults) = params.AddRelationResults{
			Endpoints: map[string]params.CharmRelation{
				"ep1": {Name: "foo"},
			},
		}
		called = true
		return nil
	})
	results, err := client.AddRelation([]string{"ep1", "ep2"}, []string{"cidr1", "cidr2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(results.Endpoints, jc.DeepEquals, map[string]params.CharmRelation{
		"ep1": {Name: "foo"},
	})
}

func (s *applicationSuite) TestGetConfigV5(c *gc.C) {
	s.assertGetConfig(c, "GetConfig", 5)
}

func (s *applicationSuite) TestGetConfigV6(c *gc.C) {
	s.assertGetConfig(c, "CharmConfig", 6)
}

func (s *applicationSuite) assertGetConfig(c *gc.C, method string, version int) {
	fooConfig := map[string]interface{}{
		"outlook": map[string]interface{}{
			"description": "No default outlook.",
			"source":      "unset",
			"type":        "string",
		},
		"skill-level": map[string]interface{}{
			"description": "A number indicating skill.",
			"source":      "user",
			"type":        "int",
			"value":       42,
		}}
	barConfig := map[string]interface{}{
		"title": map[string]interface{}{
			"default":     "My Title",
			"description": "A descriptive title used for the application.",
			"source":      "user",
			"type":        "string",
			"value":       "bar",
		},
		"username": map[string]interface{}{
			"default":     "admin001",
			"description": "The name of the initial account (given admin permissions).",
			"source":      "default",
			"type":        "string",
			"value":       "admin001",
		},
	}

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, method)
				args, ok := a.(params.Entities)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{
						{"application-foo"}, {"application-bar"},
					}})

				result, ok := response.(*params.ApplicationGetConfigResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ConfigResult{
					{Config: fooConfig}, {Config: barConfig},
				}
				return nil
			},
		),
		BestVersion: version,
	})

	results, err := client.GetConfig("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []map[string]interface{}{
		fooConfig, barConfig,
	})
}

func (s *applicationSuite) TestGetConfigAPIv4(c *gc.C) {
	fooConfig := map[string]interface{}{
		"outlook": map[string]interface{}{
			"default":     true,
			"description": "No default outlook.",
			"type":        "string",
		},
		"skill-level": map[string]interface{}{
			"description": "A number indicating skill.",
			"type":        "int",
			"value":       42,
		}}
	barConfig := map[string]interface{}{
		"title": map[string]interface{}{
			"description": "A descriptive title used for the application.",
			"type":        "string",
			"value":       "bar",
		},
		"username": map[string]interface{}{
			"default":     true,
			"description": "The name of the initial account (given admin permissions).",
			"type":        "string",
			"value":       "admin001",
		},
	}

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "Get")
				args, ok := a.(params.ApplicationGet)
				c.Assert(ok, jc.IsTrue)

				result, ok := response.(*params.ApplicationGetResults)
				c.Assert(ok, jc.IsTrue)

				switch args.ApplicationName {
				case "foo":
					result.CharmConfig = fooConfig
				case "bar":
					result.CharmConfig = barConfig
				default:
					return errors.New("unexpected app name")
				}
				return nil
			},
		),
		BestVersion: 4,
	})

	expectedFooConfig := map[string]interface{}{
		"outlook": map[string]interface{}{
			"description": "No default outlook.",
			"source":      "unset",
			"type":        "string",
		},
		"skill-level": map[string]interface{}{
			"description": "A number indicating skill.",
			"source":      "user",
			"type":        "int",
			"value":       42,
		}}
	expectedBarConfig := map[string]interface{}{
		"title": map[string]interface{}{
			// We can't infer the charm default.
			"description": "A descriptive title used for the application.",
			"source":      "user",
			"type":        "string",
			"value":       "bar",
		},
		"username": map[string]interface{}{
			"default":     "admin001",
			"description": "The name of the initial account (given admin permissions).",
			"source":      "default",
			"type":        "string",
			"value":       "admin001",
		},
	}

	results, err := client.GetConfig("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []map[string]interface{}{
		expectedFooConfig, expectedBarConfig,
	})
}

func (s *applicationSuite) TestGetConstraints(c *gc.C) {
	fooConstraints := constraints.MustParse("mem=4G")
	barConstraints := constraints.MustParse("mem=128G", "cores=64")

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "GetConstraints")
				args, ok := a.(params.Entities)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{
						{"application-foo"}, {"application-bar"},
					}})

				result, ok := response.(*params.ApplicationGetConstraintsResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ApplicationConstraint{
					{Constraints: fooConstraints}, {Constraints: barConstraints},
				}
				return nil
			},
		),
		BestVersion: 5,
	})

	results, err := client.GetConstraints("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []constraints.Value{
		fooConstraints, barConstraints,
	})
}

func (s *applicationSuite) TestGetConstraintsError(c *gc.C) {
	fooConstraints := constraints.MustParse("mem=4G")

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "GetConstraints")
				args, ok := a.(params.Entities)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{
						{"application-foo"}, {"application-bar"},
					}})

				result, ok := response.(*params.ApplicationGetConstraintsResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ApplicationConstraint{
					{Constraints: fooConstraints},
					{Error: &params.Error{Message: "oh no"}},
				}
				return nil
			},
		),
		BestVersion: 5,
	})

	results, err := client.GetConstraints("foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unable to get constraints for "bar": oh no`)
	c.Assert(results, gc.IsNil)
}

func (s *applicationSuite) TestGetConstraintsAPIv4(c *gc.C) {
	fooConstraints := constraints.MustParse("mem=4G")
	barConstraints := constraints.MustParse("mem=128G", "cores=64")

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "GetConstraints")
				args, ok := a.(params.GetApplicationConstraints)
				c.Assert(ok, jc.IsTrue)

				result, ok := response.(*params.GetConstraintsResults)
				c.Assert(ok, jc.IsTrue)

				switch args.ApplicationName {
				case "foo":
					result.Constraints = fooConstraints
				case "bar":
					result.Constraints = barConstraints
				default:
					return errors.New("unexpected app name")
				}
				return nil
			},
		),
		BestVersion: 4,
	})

	results, err := client.GetConstraints("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []constraints.Value{
		fooConstraints, barConstraints,
	})
}

func (s *applicationSuite) TestSetApplicationConfig(c *gc.C) {
	fooConfig := map[string]string{
		"foo":   "bar",
		"level": "high",
	}

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "SetApplicationsConfig")
				args, ok := a.(params.ApplicationConfigSetArgs)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.ApplicationConfigSetArgs{
					Args: []params.ApplicationConfigSet{{
						ApplicationName: "foo",
						Config:          fooConfig,
					}}})
				result, ok := response.(*params.ErrorResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ErrorResult{
					{Error: &params.Error{Message: "FAIL"}},
				}
				return nil
			},
		),
		BestVersion: 6,
	})

	err := client.SetApplicationConfig("foo", fooConfig)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestUnsetApplicationConfig(c *gc.C) {
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "UnsetApplicationsConfig")
				args, ok := a.(params.ApplicationConfigUnsetArgs)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.ApplicationConfigUnsetArgs{
					Args: []params.ApplicationUnset{{
						ApplicationName: "foo",
						Options:         []string{"option"},
					}}})
				result, ok := response.(*params.ErrorResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ErrorResult{
					{Error: &params.Error{Message: "FAIL"}},
				}
				return nil
			},
		),
		BestVersion: 6,
	})

	err := client.UnsetApplicationConfig("foo", []string{"option"})
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestSetApplicationConfigAPIv5(c *gc.C) {
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Fail()
				return errors.NotSupportedf("")
			}),
		BestVersion: 5,
	})

	err := client.SetApplicationConfig("foo", map[string]string{})
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *applicationSuite) TestUnsetApplicationConfigAPIv5(c *gc.C) {
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Fail()
				return errors.NotSupportedf("")
			}),
		BestVersion: 5,
	})

	err := client.UnsetApplicationConfig("foo", []string{})
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *applicationSuite) TestResolveUnitErrors(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(request, gc.Equals, "ResolveUnitErrors")
		args, ok := a.(params.UnitsResolved)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args, jc.DeepEquals, params.UnitsResolved{
			Retry: true,
			Tags: params.Entities{
				Entities: []params.Entity{
					{Tag: "unit-mysql-0"},
					{Tag: "unit-mysql-1"},
				},
			},
		})

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	units := []string{"mysql/0", "mysql/1"}
	err := client.ResolveUnitErrors(units, false, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestResolveUnitErrorsUnitsAll(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Fail()
		return nil
	})
	units := []string{"mysql/0"}
	err := client.ResolveUnitErrors(units, true, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "specifying units with all=true not supported")
}

func (s *applicationSuite) TestResolveUnitDuplicate(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Fail()
		return nil
	})
	units := []string{"mysql/0", "mysql/1", "mysql/0"}
	err := client.ResolveUnitErrors(units, false, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "duplicate unit specified")
}

func (s *applicationSuite) TestResolveUnitErrorsInvalidUnit(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Fail()
		return nil
	})
	units := []string{"mysql"}
	err := client.ResolveUnitErrors(units, false, false)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unit name "mysql" not valid`)
}

func (s *applicationSuite) TestResolveUnitErrorsAll(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(request, gc.Equals, "ResolveUnitErrors")
		args, ok := a.(params.UnitsResolved)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args, jc.DeepEquals, params.UnitsResolved{
			All: true,
		})

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := client.ResolveUnitErrors(nil, true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestScaleApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "ScaleApplications")
			args, ok := a.(params.ScaleApplicationsParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.ScaleApplicationsParams{
				Applications: []params.ScaleApplicationParams{
					{ApplicationTag: "application-foo", Scale: 5},
				}})

			result, ok := response.(*params.ScaleApplicationResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ScaleApplicationResult{
				{Info: &params.ScaleApplicationInfo{Scale: 5}},
			}
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	results, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 5},
	})
}

func (s *applicationSuite) TestScaleApplicationArity(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "ScaleApplications")
			args, ok := a.(params.ScaleApplicationsParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.ScaleApplicationsParams{
				Applications: []params.ScaleApplicationParams{
					{ApplicationTag: "application-foo", Scale: 5},
				}})

			result, ok := response.(*params.ScaleApplicationResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ScaleApplicationResult{
				{Info: &params.ScaleApplicationInfo{Scale: 5}},
				{Info: &params.ScaleApplicationInfo{Scale: 3}},
			}
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *applicationSuite) TestScaleApplicationError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "ScaleApplications")
			args, ok := a.(params.ScaleApplicationsParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.ScaleApplicationsParams{
				Applications: []params.ScaleApplicationParams{
					{ApplicationTag: "application-foo", Scale: 5},
				}})

			result, ok := response.(*params.ScaleApplicationResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ScaleApplicationResult{
				{Error: &params.Error{Message: "boom"}},
			}
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestScaleApplicationCallError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "ScaleApplications")
			return errors.New("boom")
		},
	)
	client := application.NewClient(apiCaller)
	_, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		Scale:           5,
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestSetCharmProfileError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "SetCharmProfile")
			return errors.New("boom")
		},
	)
	client := newClient(apiCaller)
	err := client.SetCharmProfile("foo", charmstore.CharmID{
		URL: charm.MustParseURL("local:testing-1"),
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}
