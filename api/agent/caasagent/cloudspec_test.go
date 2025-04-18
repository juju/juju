// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/caasagent"
	apitesting "github.com/juju/juju/api/base/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = gc.Suite(&ClientSuite{})

type ClientSuite struct {
	testing.IsolationSuite
}

func (s *ClientSuite) TestWatchCloudSpecChanges(c *gc.C) {
	called := false
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASAgent")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchCloudSpecsChanges")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: internaltesting.ModelTag.String(),
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "666",
			}},
		}
		called = true
		return nil
	})

	api, err := caasagent.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	w, err := api.WatchCloudSpecChanges(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	c.Assert(called, jc.IsTrue)
}
