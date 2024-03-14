// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/types"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type modelSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestModel(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(id, gc.Equals, "")
		switch request {
		case "CurrentModel":
			c.Assert(arg, gc.IsNil)
			c.Assert(result, gc.FitsTypeOf, &params.ModelResult{})
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
