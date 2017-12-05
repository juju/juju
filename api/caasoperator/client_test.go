// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	names "gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/apiserver/params"
)

type operatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&operatorSuite{})

func (s *operatorSuite) TestSetStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetStatus")
		c.Check(arg, jc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "foo",
				Info:   "bar",
				Data: map[string]interface{}{
					"baz": "qux",
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetStatus("gitlab", "foo", "bar", map[string]interface{}{
		"baz": "qux",
	})
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestSetStatusInvalidApplicationName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetStatus("", "foo", "bar", nil)
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestCharm(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Charm")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ApplicationCharmResults{})
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{
				Result: &params.ApplicationCharm{
					URL:          "cs:foo/bar-1",
					ForceUpgrade: true,
					SHA256:       "fake-sha256",
				},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	curl, sha256, err := client.Charm("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, "cs:foo/bar-1")
	c.Assert(sha256, gc.Equals, "fake-sha256")
}

func (s *operatorSuite) TestCharmError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})
	client := caasoperator.NewClient(apiCaller)
	_, _, err := client.Charm("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestCharmInvalidApplicationName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, _, err := client.Charm("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestApplicationConfig(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ApplicationConfig")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ConfigSettingsResults{})
		*(result.(*params.ConfigSettingsResults)) = params.ConfigSettingsResults{
			Results: []params.ConfigSettingsResult{{
				Settings: params.ConfigSettings{"k": 123},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	settings, err := client.ApplicationConfig("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, charm.Settings{"k": 123})
}

func (s *operatorSuite) TestWatchApplicationConfig(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplicationConfig")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	watcher, err := client.WatchApplicationConfig("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *operatorSuite) TestSetContainerSpec(c *gc.C) {
	s.testSetContainerSpec(c, names.NewApplicationTag("gitlab"))
	s.testSetContainerSpec(c, names.NewUnitTag("gitlab/0"))
}

func (s *operatorSuite) testSetContainerSpec(c *gc.C, tag names.Tag) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetContainerSpec")
		c.Check(arg, jc.DeepEquals, params.SetContainerSpecParams{
			Entities: []params.EntityString{{
				Tag:   tag.String(),
				Value: "spec",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetContainerSpec(tag.Id(), "spec")
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestSetContainerSpecInvalidEntityame(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetContainerSpec("", "spec")
	c.Assert(err, gc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *operatorSuite) TestModelName(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ModelName")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringResult{})
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "some-model",
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	name, err := client.ModelName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "some-model")
}

func (s *operatorSuite) TestAPIAddresses(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "APIAddresses")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsResult{})
		*(result.(*params.StringsResult)) = params.StringsResult{
			Result: []string{"10.0.0.1:10000"},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	addresses, err := client.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.DeepEquals, []string{"10.0.0.1:10000"})
}

func (s *operatorSuite) TestProxySettings(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ProxyConfig")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ProxyConfig{})
		*(result.(*params.ProxyConfig)) = params.ProxyConfig{
			HTTP:    "http.proxy",
			HTTPS:   "https.proxy",
			FTP:     "ftp.proxy",
			NoProxy: "no.proxy",
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	settings, err := client.ProxySettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, proxy.Settings{
		Http:    "http.proxy",
		Https:   "https.proxy",
		Ftp:     "ftp.proxy",
		NoProxy: "no.proxy",
	})
}
