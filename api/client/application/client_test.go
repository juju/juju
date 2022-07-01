// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stderrors "errors"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/v2/api/base/testing"
	"github.com/juju/juju/v2/api/client/application"
	apicharm "github.com/juju/juju/v2/api/common/charm"
	apitesting "github.com/juju/juju/v2/api/testing"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/core/constraints"
	"github.com/juju/juju/v2/core/crossmodel"
	"github.com/juju/juju/v2/core/instance"
	"github.com/juju/juju/v2/core/model"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/storage"
	coretesting "github.com/juju/juju/v2/testing"
)

const newBranchName = "new-branch"

type applicationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&applicationSuite{})

func newClient(f basetesting.APICallerFunc) *application.Client {
	return newClientWithVersion(f, 13)
}

func newClientWithVersion(f basetesting.APICallerFunc, version int) *application.Client {
	return application.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: version})
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
		result.Results[0].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
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
				c.Assert(app.CharmOrigin, gc.DeepEquals, &params.CharmOrigin{Source: "charm-store"})
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
		CharmID: application.CharmID{
			URL: charm.MustParseURL("cs:trusty/a-charm-1"),
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmStore,
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

func (s *applicationSuite) TestDeployAlreadyExists(c *gc.C) {
	var called bool
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				called = true
				c.Assert(request, gc.Equals, "Deploy")

				result := response.(*params.ErrorResults)
				result.Results = []params.ErrorResult{
					{Error: &params.Error{
						Message: "application already exists",
						Code:    params.CodeAlreadyExists,
					}},
				}
				return nil
			},
		),
		BestVersion: 5,
	})

	args := application.DeployArgs{
		CharmID: application.CharmID{
			URL: charm.MustParseURL("cs:trusty/a-charm-1"),
		},
		CharmOrigin: apicharm.Origin{
			Source: apicharm.OriginCharmStore,
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
	c.Assert(err, gc.ErrorMatches, `application already exists`)
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
		c.Assert(args.BranchName, gc.Equals, newBranchName)

		result := response.(*params.StringResult)
		result.Result = "cs:curl"
		return nil
	})
	curl, err := client.GetCharmURL(newBranchName, "application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("cs:curl"))
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationGetCharmURLOrigin(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "GetCharmURLOrigin")
		args, ok := a.(params.ApplicationGet)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")
		c.Assert(args.BranchName, gc.Equals, newBranchName)

		result := response.(*params.CharmURLOriginResult)
		result.URL = "cs:curl"
		result.Origin = params.CharmOrigin{
			Risk: "edge",
		}
		return nil
	})
	curl, origin, err := client.GetCharmURLOrigin(newBranchName, "application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("cs:curl"))
	c.Assert(origin, gc.DeepEquals, apicharm.Origin{
		Risk: "edge",
	})
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationGetCharmURLOriginWithOlderAPIVersion(c *gc.C) {
	var called bool
	client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "GetCharmURL")
		args, ok := a.(params.ApplicationGet)
		c.Assert(ok, jc.IsTrue)
		c.Assert(args.ApplicationName, gc.Equals, "application")
		c.Assert(args.BranchName, gc.Equals, newBranchName)

		result := response.(*params.StringResult)
		result.Result = "cs:curl"
		return nil
	}, 12)
	curl, origin, err := client.GetCharmURLOrigin(newBranchName, "application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, charm.MustParseURL("cs:curl"))
	c.Assert(origin, gc.DeepEquals, apicharm.Origin{
		Source: apicharm.OriginCharmStore,
	})
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
		c.Assert(args.CharmOrigin, gc.DeepEquals, &params.CharmOrigin{
			Source: "charm-hub",
			Risk:   "edge",
		})
		c.Assert(args.ConfigSettings, jc.DeepEquals, map[string]string{
			"a": "b",
			"c": "d",
		})
		c.Assert(args.ConfigSettingsYAML, gc.Equals, "yaml")
		c.Assert(args.Force, gc.Equals, true)
		c.Assert(args.ForceSeries, gc.Equals, true)
		c.Assert(args.ForceUnits, gc.Equals, true)
		c.Assert(args.StorageConstraints, jc.DeepEquals, map[string]params.StorageConstraints{
			"a": {Pool: "radiant"},
			"b": {Count: toUint64Ptr(123)},
			"c": {Size: toUint64Ptr(123)},
		})
		c.Assert(args.Generation, gc.Equals, newBranchName)

		return nil
	})
	cfg := application.SetCharmConfig{
		ApplicationName: "application",
		CharmID: application.CharmID{
			URL: charm.MustParseURL("cs:trusty/application-1"),
			Origin: apicharm.Origin{
				Source: "charm-hub",
				Risk:   "edge",
			},
		},
		ConfigSettings: map[string]string{
			"a": "b",
			"c": "d",
		},
		ConfigSettingsYAML: "yaml",
		Force:              true,
		ForceSeries:        true,
		ForceUnits:         true,
		StorageConstraints: map[string]storage.Constraints{
			"a": {Pool: "radiant"},
			"b": {Count: 123},
			"c": {Size: 123},
		},
	}
	err := client.SetCharm(newBranchName, cfg)
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
	delay := 1 * time.Minute
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyApplication")
		c.Assert(a, jc.DeepEquals, params.DestroyApplicationsParams{
			Applications: []params.DestroyApplicationParams{
				{ApplicationTag: "application-foo", Force: true, MaxWait: &delay},
				{ApplicationTag: "application-bar", Force: true, MaxWait: &delay},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyApplicationResults{})
		out := response.(*params.DestroyApplicationResults)
		*out = params.DestroyApplicationResults{expectedResults}
		return nil
	})
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"foo", "bar"},
		Force:        true,
		MaxWait:      &delay,
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
	client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
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
	}, 4) // use version 4
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

func (s *applicationSuite) TestDestroyConsumedApplicationsV8(c *gc.C) {
	expectedResults := []params.ErrorResult{{
		Error: &params.Error{Message: "boo"},
	}, {}}
	client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
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
	}, 8) // use V8
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo", "bar"}, false, nil,
	}
	results, err := client.DestroyConsumedApplication(destroyParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, nil,
	}
	_, err := client.DestroyConsumedApplication(destroyParams)
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsForcev9(c *gc.C) {
	var called bool
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "DestroyConsumedApplications")
				c.Assert(a, jc.DeepEquals, params.DestroyConsumedApplicationsParams{
					Applications: []params.DestroyConsumedApplicationParams{
						{ApplicationTag: "application-foo"}, // check that Force and MaxWait are not supplied to the controller
					},
				})
				called = true
				return nil
			},
		),
		BestVersion: 9, // v9 does not support --force or --no-wait
	})
	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, true, nil,
	}
	results, err := client.DestroyConsumedApplication(destroyParams)
	c.Check(err, gc.ErrorMatches, "this controller does not support --force")
	c.Check(results, gc.HasLen, 0)
	c.Assert(called, jc.IsFalse)

	noWait := time.Minute * 0
	destroyParams = application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, &noWait,
	}
	results, err = client.DestroyConsumedApplication(destroyParams)
	c.Check(err, gc.ErrorMatches, "this controller does not support --no-wait")
	c.Check(results, gc.HasLen, 0)
	c.Assert(called, jc.IsFalse)

	destroyParams = application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, nil,
	}
	_, err = client.DestroyConsumedApplication(destroyParams)
	c.Check(err, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestDestroyConsumedApplicationsForcev10(c *gc.C) {
	var called bool
	noWait := 0 * time.Minute
	force := true
	expectedResults := []params.ErrorResult{{}, {}}
	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "DestroyConsumedApplications")
				c.Assert(a, jc.DeepEquals, params.DestroyConsumedApplicationsParams{
					Applications: []params.DestroyConsumedApplicationParams{
						{ApplicationTag: "application-foo", Force: &force, MaxWait: &noWait},
						{ApplicationTag: "application-bar", Force: &force, MaxWait: &noWait},
					},
				})
				called = true
				c.Assert(response, gc.FitsTypeOf, &params.ErrorResults{})
				out := response.(*params.ErrorResults)
				*out = params.ErrorResults{expectedResults}
				return nil
			},
		),
		BestVersion: 10,
	})

	destroyParams := application.DestroyConsumedApplicationParams{
		[]string{"foo"}, false, &noWait,
	}
	results, err := client.DestroyConsumedApplication(destroyParams)
	c.Check(err, gc.ErrorMatches, "--force is required when --max-wait is provided")
	c.Check(results, gc.HasLen, 0)
	c.Assert(called, jc.IsFalse)

	destroyParams = application.DestroyConsumedApplicationParams{
		[]string{"foo", "bar"}, force, &noWait,
	}
	results, err = client.DestroyConsumedApplication(destroyParams)
	c.Check(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 2)
	c.Assert(called, jc.IsTrue)
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
	delay := 1 * time.Minute
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyUnit")
		c.Assert(a, jc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{UnitTag: "unit-foo-0", Force: true, MaxWait: &delay},
				{UnitTag: "unit-bar-1", Force: true, MaxWait: &delay},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyUnitResults{})
		out := response.(*params.DestroyUnitResults)
		*out = params.DestroyUnitResults{expectedResults}
		return nil
	})
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:   []string{"foo/0", "bar/1"},
		Force:   true,
		MaxWait: &delay,
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
	client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
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
	}, 4) // use V4
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
	false_ := false
	true_ := true
	zero := time.Minute * 1
	for _, t := range []struct {
		force   *bool
		maxWait *time.Duration
	}{
		{},
		{force: &true_},
		{force: &false_},
		{maxWait: &zero},
		{force: &false_, maxWait: &zero},
		{force: &true_, maxWait: &zero},
	} {
		called := false
		client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "DestroyRelation")
			c.Assert(a, jc.DeepEquals, params.DestroyRelation{
				Endpoints: []string{"ep1", "ep2"},
				Force:     t.force,
				MaxWait:   t.maxWait,
			})
			c.Assert(response, gc.IsNil)
			called = true
			return nil
		})

		err := client.DestroyRelation(t.force, t.maxWait, "ep1", "ep2")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(called, jc.IsTrue)
	}
}

func (s *applicationSuite) TestDestroyRelationId(c *gc.C) {
	false_ := false
	true_ := true
	zero := time.Minute * 1
	for _, t := range []struct {
		force   *bool
		maxWait *time.Duration
	}{
		{},
		{force: &true_},
		{force: &false_},
		{maxWait: &zero},
		{force: &false_, maxWait: &zero},
		{force: &true_, maxWait: &zero},
	} {
		called := false
		client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "DestroyRelation")
			c.Assert(a, jc.DeepEquals, params.DestroyRelation{
				RelationId: 123,
				Force:      t.force,
				MaxWait:    t.maxWait,
			})
			c.Assert(response, gc.IsNil)
			called = true
			return nil
		})
		err := client.DestroyRelationId(123, t.force, t.maxWait)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(called, jc.IsTrue)
	}
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
	args := params.Entities{Entities: []params.Entity{{"application-foo"}, {"application-bar"}}}
	s.assertGetConfig(c, 5, "GetConfig", args)
}

func (s *applicationSuite) TestGetConfigV6(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{"application-foo"}, {"application-bar"}}}
	s.assertGetConfig(c, 6, "CharmConfig", args)
}

func (s *applicationSuite) TestGetConfigV9(c *gc.C) {
	args := params.ApplicationGetArgs{Args: []params.ApplicationGet{
		{ApplicationName: "foo", BranchName: newBranchName},
		{ApplicationName: "bar", BranchName: newBranchName},
	}}
	s.assertGetConfig(c, 9, "CharmConfig", args)
}

func (s *applicationSuite) assertGetConfig(c *gc.C, version int, method string, expArgs interface{}) {
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
			func(objType string, version int, id, request string, args, response interface{}) error {
				c.Assert(request, gc.Equals, method)
				c.Assert(args, jc.DeepEquals, expArgs)

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

	results, err := client.GetConfig(newBranchName, "foo", "bar")
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
				c.Assert(args.BranchName, gc.Equals, newBranchName)

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

	results, err := client.GetConfig(newBranchName, "foo", "bar")
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
						Generation:      newBranchName,
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

	err := client.SetApplicationConfig(newBranchName, "foo", fooConfig)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestSetApplicationConfigNoSupported(c *gc.C) {
	fooConfig := map[string]string{
		"foo":   "bar",
		"level": "high",
	}

	client := application.NewClient(basetesting.BestVersionCaller{
		BestVersion: 13,
	})

	err := client.SetApplicationConfig(newBranchName, "foo", fooConfig)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *applicationSuite) TestSetConfig(c *gc.C) {
	fooConfig := map[string]string{
		"foo":   "bar",
		"level": "high",
	}
	fooConfigYaml := "foo"

	client := application.NewClient(basetesting.BestVersionCaller{
		APICallerFunc: basetesting.APICallerFunc(
			func(objType string, version int, id, request string, a, response interface{}) error {
				c.Assert(request, gc.Equals, "SetConfigs")
				args, ok := a.(params.ConfigSetArgs)
				c.Assert(ok, jc.IsTrue)
				c.Assert(args, jc.DeepEquals, params.ConfigSetArgs{
					Args: []params.ConfigSet{{
						ApplicationName: "foo",
						Config:          fooConfig,
						ConfigYAML:      fooConfigYaml,
						Generation:      newBranchName,
					}}})
				result, ok := response.(*params.ErrorResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.ErrorResult{
					{Error: &params.Error{Message: "FAIL"}},
				}
				return nil
			},
		),
		BestVersion: 13,
	})

	err := client.SetConfig(newBranchName, "foo", fooConfigYaml, fooConfig)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *applicationSuite) TestSetConfigNotSupported(c *gc.C) {
	fooConfig := map[string]string{
		"foo":   "bar",
		"level": "high",
	}
	fooConfigYaml := "foo"

	client := application.NewClient(basetesting.BestVersionCaller{
		BestVersion: 12,
	})

	err := client.SetConfig(newBranchName, "foo", fooConfigYaml, fooConfig)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
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
						BranchName:      newBranchName,
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

	err := client.UnsetApplicationConfig(newBranchName, "foo", []string{"option"})
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

	err := client.SetApplicationConfig(model.GenerationMaster, "foo", map[string]string{})
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

	err := client.UnsetApplicationConfig(model.GenerationMaster, "foo", []string{})
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
					{ApplicationTag: "application-foo", Scale: 5, Force: true},
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
		Force:           true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 5},
	})
}

func (s *applicationSuite) TestChangeScaleApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "ScaleApplications")
			args, ok := a.(params.ScaleApplicationsParams)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.ScaleApplicationsParams{
				Applications: []params.ScaleApplicationParams{
					{ApplicationTag: "application-foo", ScaleChange: 5},
				}})

			result, ok := response.(*params.ScaleApplicationResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ScaleApplicationResult{
				{Info: &params.ScaleApplicationInfo{Scale: 7}},
			}
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	results, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: "foo",
		ScaleChange:     5,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{Scale: 7},
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

func (s *applicationSuite) TestScaleApplicationValidation(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			return nil
		},
	)
	client := application.NewClient(apiCaller)

	for i, test := range []struct {
		scale       int
		scaleChange int
		errorStr    string
	}{{
		scale:       5,
		scaleChange: 5,
		errorStr:    "requesting both scale and scale-change not valid",
	}, {
		scale:       -1,
		scaleChange: 0,
		errorStr:    "scale < 0 not valid",
	}} {
		c.Logf("test #%d", i)
		_, err := client.ScaleApplication(application.ScaleApplicationParams{
			ApplicationName: "foo",
			Scale:           test.scale,
			ScaleChange:     test.scaleChange,
		})
		c.Assert(err, gc.ErrorMatches, test.errorStr)
	}
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

func (s *applicationSuite) TestApplicationsInfoPriorV9(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "ApplicationsInfo")
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	_, err := client.ApplicationsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "ApplicationsInfo for Application facade v0 not supported")
	c.Assert(called, jc.IsFalse)
}

func apiForApplicationsInfo(f basetesting.APICallerFunc) basetesting.BestVersionCaller {
	return basetesting.BestVersionCaller{
		BestVersion:   9,
		APICallerFunc: f,
	}
}

func (s *applicationSuite) TestApplicationsInfoCallError(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "ApplicationsInfo")
			return errors.New("boom")
		},
	)

	client := application.NewClient(apiForApplicationsInfo(apiCaller))
	_, err := client.ApplicationsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationsInfo(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "ApplicationsInfo")
			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{
					{Tag: "application-foo"},
					{Tag: "application-bar"},
				}})

			result, ok := response.(*params.ApplicationInfoResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ApplicationInfoResult{
				{Error: &params.Error{Message: "boom"}},
				{Result: &params.ApplicationResult{
					Tag:       "application-bar",
					Charm:     "charm-bar",
					Series:    "bionic",
					Channel:   "development",
					Principal: true,
					EndpointBindings: map[string]string{
						"juju-info": "myspace",
					},
					Remote: true,
				},
				},
			}
			return nil
		},
	)

	client := application.NewClient(apiForApplicationsInfo(apiCaller))
	results, err := client.ApplicationsInfo(
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Check(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []params.ApplicationInfoResult{
		{Error: &params.Error{Message: "boom"}},
		{Result: &params.ApplicationResult{
			Tag:       "application-bar",
			Charm:     "charm-bar",
			Series:    "bionic",
			Channel:   "development",
			Principal: true,
			EndpointBindings: map[string]string{
				"juju-info": "myspace",
			},
			Remote: true,
		}},
	})
}

func (s *applicationSuite) TestApplicationsInfoResultMismatch(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "ApplicationsInfo")

			result, ok := response.(*params.ApplicationInfoResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.ApplicationInfoResult{
				{Error: &params.Error{Message: "boom"}},
				{Error: &params.Error{Message: "boom again"}},
				{Result: &params.ApplicationResult{Tag: "application-bar"}},
			}
			return nil
		},
	)

	client := application.NewClient(apiForApplicationsInfo(apiCaller))
	_, err := client.ApplicationsInfo(
		[]names.ApplicationTag{
			names.NewApplicationTag("foo"),
			names.NewApplicationTag("bar"),
		},
	)
	c.Check(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestUnitsInfoBotSupported(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "UnitsInfo")
			return nil
		},
	)
	client := application.NewClient(apiCaller)
	_, err := client.UnitsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "UnitsInfo for Application facade v0 not supported")
	c.Assert(called, jc.IsFalse)
}

func apiForUnitsInfo(f basetesting.APICallerFunc) basetesting.BestVersionCaller {
	return basetesting.BestVersionCaller{
		BestVersion:   12,
		APICallerFunc: f,
	}
}

func (s *applicationSuite) TestUnitsInfoCallError(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "UnitsInfo")
			return errors.New("boom")
		},
	)

	client := application.NewClient(apiForUnitsInfo(apiCaller))
	_, err := client.UnitsInfo(nil)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(called, jc.IsTrue)
}

func (s *applicationSuite) TestUnitsInfo(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "UnitsInfo")
			args, ok := a.(params.Entities)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args, jc.DeepEquals, params.Entities{
				Entities: []params.Entity{
					{Tag: "unit-foo-0"},
					{Tag: "unit-bar-1"},
				}})

			result, ok := response.(*params.UnitInfoResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.UnitInfoResult{
				{Error: &params.Error{Message: "boom"}},
				{Result: &params.UnitResult{
					Tag:             "unit-bar-1",
					WorkloadVersion: "666",
					Machine:         "1",
					OpenedPorts:     []string{"80"},
					PublicAddress:   "10.0.0.1",
					Charm:           "charm-bar",
					Leader:          true,
					RelationData: []params.EndpointRelationData{{
						Endpoint:        "db",
						CrossModel:      true,
						RelatedEndpoint: "server",
						ApplicationData: map[string]interface{}{"foo": "bar"},
						UnitRelationData: map[string]params.RelationData{
							"baz": {
								InScope:  true,
								UnitData: map[string]interface{}{"hello": "world"},
							},
						},
					}},
					ProviderId: "provider-id",
					Address:    "192.168.1.1",
				}},
			}
			return nil
		},
	)

	client := application.NewClient(apiForUnitsInfo(apiCaller))
	results, err := client.UnitsInfo(
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Check(called, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []application.UnitInfo{
		{Error: stderrors.New("boom")},
		{
			Tag:             "unit-bar-1",
			WorkloadVersion: "666",
			Machine:         "1",
			OpenedPorts:     []string{"80"},
			PublicAddress:   "10.0.0.1",
			Charm:           "charm-bar",
			Leader:          true,
			RelationData: []application.EndpointRelationData{{
				Endpoint:        "db",
				CrossModel:      true,
				RelatedEndpoint: "server",
				ApplicationData: map[string]interface{}{"foo": "bar"},
				UnitRelationData: map[string]application.RelationData{
					"baz": {
						InScope:  true,
						UnitData: map[string]interface{}{"hello": "world"},
					},
				},
			}},
			ProviderId: "provider-id",
			Address:    "192.168.1.1",
		},
	})
}

func (s *applicationSuite) TestUnitsInfoResultMismatch(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, response interface{}) error {
			c.Assert(request, gc.Equals, "UnitsInfo")

			result, ok := response.(*params.UnitInfoResults)
			c.Assert(ok, jc.IsTrue)
			result.Results = []params.UnitInfoResult{
				{}, {}, {},
			}
			return nil
		},
	)

	client := application.NewClient(apiForUnitsInfo(apiCaller))
	_, err := client.UnitsInfo(
		[]names.UnitTag{
			names.NewUnitTag("foo/0"),
			names.NewUnitTag("bar/1"),
		},
	)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 3")
}

func (s *applicationSuite) TestExposeVersionChecks(c *gc.C) {
	specs := []struct {
		descr            string
		facadeVersion    int
		exposedEndpoints map[string]params.ExposedEndpoint
		expErr           string
	}{
		{
			descr:         "use expose parameters with pre 2.9 controller",
			facadeVersion: 12,
			exposedEndpoints: map[string]params.ExposedEndpoint{
				"foo": {
					ExposeToSpaces: []string{"outer"},
				},
			},
			expErr: "controller does not support granular expose parameters; applying this change would make all open application ports accessible from 0.0.0.0/0",
		},
		{
			descr:         "use expose parameters with pre 2.9 controller but expose all endpoints to 0.0.0.0/0",
			facadeVersion: 12,
			exposedEndpoints: map[string]params.ExposedEndpoint{
				"": {
					ExposeToCIDRs: []string{"0.0.0.0/0"},
				},
			},
			expErr: "",
		},
		{
			descr:         "use expose parameters with pre 2.9 controller but expose all endpoints to 0.0.0.0/0 and ::/0",
			facadeVersion: 12,
			exposedEndpoints: map[string]params.ExposedEndpoint{
				"": {
					ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
				},
			},
			expErr: "",
		},
		{
			descr:            "don't use expose parameters",
			facadeVersion:    12,
			exposedEndpoints: nil,
			expErr:           "",
		},
		{
			descr:         "use expose parameters with 2.9 controller",
			facadeVersion: 13,
			exposedEndpoints: map[string]params.ExposedEndpoint{
				"": {
					ExposeToCIDRs: []string{"0.0.0.0/0"},
				},
				"foo": {
					ExposeToSpaces: []string{"outer"},
				},
			},
			expErr: "",
		},
	}

	for i, spec := range specs {
		c.Logf("%d. %s", i, spec.descr)

		client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
			return nil
		}, spec.facadeVersion)

		err := client.Expose("foo", spec.exposedEndpoints)
		if spec.expErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, spec.expErr)
		}
	}
}

func (s *applicationSuite) TestUnexposeVersionChecks(c *gc.C) {
	specs := []struct {
		descr            string
		facadeVersion    int
		exposedEndpoints []string
		expErr           string
	}{
		{
			descr:            "use exposed endpoints with pre 2.9 controller",
			facadeVersion:    12,
			exposedEndpoints: []string{"foo"},
			expErr:           "controller does not support granular expose parameters; applying this change would unexpose the application",
		},
		{
			descr:            "don't use expose parameters",
			facadeVersion:    12,
			exposedEndpoints: nil,
			expErr:           "",
		},
		{
			descr:            "use exposed endpoints with 2.9 controller",
			facadeVersion:    13,
			exposedEndpoints: []string{"foo"},
			expErr:           "",
		},
	}

	for i, spec := range specs {
		c.Logf("%d. %s", i, spec.descr)

		client := newClientWithVersion(func(objType string, version int, id, request string, a, response interface{}) error {
			return nil
		}, spec.facadeVersion)

		err := client.Unexpose("foo", spec.exposedEndpoints)
		if spec.expErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, spec.expErr)
		}
	}
}
