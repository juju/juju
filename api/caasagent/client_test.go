// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasagent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/testing"
)

type agentSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&agentSuite{})

func (s *agentSuite) TestModel(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASAgent")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Model")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.Model{})
		*(result.(*params.Model)) = params.Model{
			Name:     "mymodel",
			UUID:     coretesting.ModelTag.Id(),
			Type:     "iaas",
			OwnerTag: "user-fred",
		}
		return nil
	})

	client, err := caasagent.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	result, err := client.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &model.Model{
		Name:  "mymodel",
		UUID:  coretesting.ModelTag.Id(),
		Type:  "iaas",
		Owner: names.NewUserTag("fred"),
	})
}
