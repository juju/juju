// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasprovisioner"
	"github.com/juju/juju/apiserver/params"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasprovisioner.Client {
	return caasprovisioner.NewClient(basetesting.BestVersionCaller{f, 5})
}

func (s *provisionerSuite) TestConnectionConfig(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ConnectionConfig")
		c.Assert(a, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.CAASConnectionConfig{})
		*(result.(*params.CAASConnectionConfig)) = params.CAASConnectionConfig{
			Endpoint:       "endpoint",
			Username:       "fred",
			Password:       "password",
			CACertificates: []string{"cert"},
			CertData:       []byte("certdata"),
			KeyData:        []byte("keydata"),
		}
		return nil
	})
	config, err := client.ConnectionConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(config, jc.DeepEquals, &caasprovisioner.ConnectionConfig{
		Endpoint:       "endpoint",
		Username:       "fred",
		Password:       "password",
		CACertificates: []string{"cert"},
		CertData:       []byte("certdata"),
		KeyData:        []byte("keydata"),
	})
}

func (s *provisionerSuite) TestWatchApplications(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "WatchApplications")
		c.Assert(a, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.WatchApplications()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(called, jc.IsTrue)
}
