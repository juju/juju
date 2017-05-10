// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/application"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

type applicationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&applicationSuite{})

func newClient(f basetesting.APICallerFunc) *application.Client {
	return application.NewClient(f)
}

func (s *applicationSuite) TestSetServiceMetricCredentials(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "Application")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetMetricCredentials")
		args, ok := a.(params.ApplicationMetricCredentials)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Creds, gc.HasLen, 1)
		c.Assert(args.Creds[0].ApplicationName, gc.Equals, "serviceA")
		c.Assert(args.Creds[0].MetricCredentials, gc.DeepEquals, []byte("creds 1"))

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})
	err := client.SetMetricCredentials("serviceA", []byte("creds 1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestSetServiceMetricCredentialsFails(c *gc.C) {
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
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "Deploy")
		args, ok := a.(params.ApplicationsDeploy)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Applications, gc.HasLen, 1)
		app := args.Applications[0]
		c.Assert(app.CharmURL, gc.Equals, "cs:trusty/a-charm-1")
		c.Assert(app.ApplicationName, gc.Equals, "serviceA")
		c.Assert(app.Series, gc.Equals, "series")
		c.Assert(app.NumUnits, gc.Equals, 1)
		c.Assert(app.ConfigYAML, gc.Equals, "configYAML")
		c.Assert(app.Constraints, gc.DeepEquals, constraints.MustParse("mem=4G"))
		c.Assert(app.Placement, gc.DeepEquals, []*instance.Placement{{"scope", "directive"}})
		c.Assert(app.EndpointBindings, gc.DeepEquals, map[string]string{"foo": "bar"})
		c.Assert(app.Storage, gc.DeepEquals, map[string]storage.Constraints{"data": storage.Constraints{Pool: "pool"}})
		c.Assert(app.AttachStorage, gc.DeepEquals, []string{"storage-data-0"})
		c.Assert(app.Resources, gc.DeepEquals, map[string]string{"foo": "bar"})

		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})

	args := application.DeployArgs{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("trusty/a-charm-1"),
		},
		ApplicationName:  "serviceA",
		Series:           "series",
		NumUnits:         1,
		ConfigYAML:       "configYAML",
		Cons:             constraints.MustParse("mem=4G"),
		Placement:        []*instance.Placement{{"scope", "directive"}},
		Storage:          map[string]storage.Constraints{"data": storage.Constraints{Pool: "pool"}},
		AttachStorage:    []string{"data/0"},
		Resources:        map[string]string{"foo": "bar"},
		EndpointBindings: map[string]string{"foo": "bar"},
	}
	err := client.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
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
	c.Assert(err, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
	c.Assert(called, jc.IsFalse)
}

func (s *applicationSuite) TestServiceGetCharmURL(c *gc.C) {
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

func (s *applicationSuite) TestServiceSetCharm(c *gc.C) {
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

func (s *applicationSuite) TestConsume(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "Consume")
		args, ok := a.(params.ConsumeApplicationArgs)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.Args, jc.DeepEquals, []params.ConsumeApplicationArg{
			{ApplicationURL: "remote app url", ApplicationAlias: "alias"},
		})
		result := response.(*params.ConsumeApplicationResults)
		result.Results = []params.ConsumeApplicationResult{{LocalName: "result"}}
		return nil
	})
	name, err := client.Consume("remote app url", "alias")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "result")
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
	results, err := client.DestroyApplications("foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyApplicationsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyApplications("foo")
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
	results, err := client.DestroyApplications("!", "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
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
	results, err := client.DestroyUnits("foo/0", "bar/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyUnitsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyUnits("foo/0")
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
	results, err := client.DestroyUnits("!", "foo/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}
