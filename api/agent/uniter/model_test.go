// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
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
	client := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
	m, err := client.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, &model.Model{
		Name:      "mary",
		UUID:      "deadbeaf",
		ModelType: model.CAAS,
	})
}
