// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/types"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type modelSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&modelSuite{})

func (s *modelSuite) TestModel(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(id, tc.Equals, "")
		switch request {
		case "CurrentModel":
			c.Assert(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.ModelResult{})
			*(result.(*params.ModelResult)) = params.ModelResult{
				Name: "mary",
				UUID: "deadbeaf",
				Type: "caas",
			}
		default:
			c.Fatalf("unexpected api call %q", request)
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	m, err := client.Model(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, &types.Model{
		Name:      "mary",
		UUID:      "deadbeaf",
		ModelType: types.CAAS,
	})
}
