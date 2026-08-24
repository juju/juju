// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/api/agent/caasagent"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestClientSuite(t *testing.T) {
	tc.Run(t, &ClientSuite{})
}

type ClientSuite struct {
	testhelpers.IsolationSuite
}

func (s *ClientSuite) TestWatchCloudSpecChanges(c *tc.C) {
	called := false
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		// We might get a second call to "Next" but
		// we don't care.
		if called {
			return nil
		}
		c.Check(objType, tc.Equals, "CAASAgent")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchCloudSpecsChanges")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: internaltesting.ModelTag.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "666",
			}},
		}
		called = true
		return nil
	})

	api, err := caasagent.NewClient(apiCaller)
	c.Assert(err, tc.ErrorIsNil)
	w, err := api.WatchCloudSpecChanges(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	workertest.CleanKill(c, w)
}

func (s *ClientSuite) TestAPIAddressClient(c *tc.C) {
	called := false
	apiCaller := apitesting.BestVersionCaller{
		APICallerFunc: func(objType string, version int, id, request string, arg, result any) error {
			c.Check(objType, tc.Equals, "CAASAgent")
			c.Check(version, tc.Equals, 3)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "APIHostPorts")
			c.Check(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.APIHostPortsResult{})
			*(result.(*params.APIHostPortsResult)) = params.APIHostPortsResult{
				Servers: params.FromHostsPorts([]network.HostPorts{
					network.NewMachineHostPorts(17070, "10.0.0.1").HostPorts(),
				}),
			}
			called = true
			return nil
		},
		BestVersion: 3,
	}

	addresses, err := caasagent.NewAPIAddressClient(apiCaller).APIHostPorts(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(addresses, tc.DeepEquals, []network.ProviderHostPorts{{
		{ProviderAddress: network.NewMachineAddress("10.0.0.1").AsProviderAddress(), NetPort: 17070},
	}})
}
