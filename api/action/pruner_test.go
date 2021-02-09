// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/action"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
)

type prunerSuite struct{}

var _ = gc.Suite(&prunerSuite{})

func (s *prunerSuite) TestPrune(c *gc.C) {
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Assert(request, gc.Equals, "Prune")
			c.Assert(a, jc.DeepEquals, params.ActionPruneArgs{
				MaxHistoryTime: time.Hour,
				MaxHistoryMB:   666,
			})
			c.Assert(result, gc.IsNil)
			called = true
			return nil
		},
	)
	client := action.NewPruner(apiCaller)
	err := client.Prune(time.Hour, 666)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
