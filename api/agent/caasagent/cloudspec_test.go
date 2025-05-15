// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api/agent/caasagent"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&ClientSuite{})

type ClientSuite struct {
	testhelpers.IsolationSuite
}

func (s *ClientSuite) TestWatchCloudSpecChanges(c *tc.C) {
	called := false
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
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
