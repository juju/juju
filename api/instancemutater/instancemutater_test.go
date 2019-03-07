// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

type instanceMutaterSuite struct {
	jujutesting.BaseSuite

	tag names.Tag
}

var _ = gc.Suite(&instanceMutaterSuite{})

func (s *instanceMutaterSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.BaseSuite.SetUpTest(c)
}

func (s *instanceMutaterSuite) TestWatchModelMachines(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "WatchModelMachines")
		c.Assert(args, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{
				{Tag: s.tag.String()},
			},
		})
		*(response.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "4",
				Changes:          []string{"0"},
				Error:            nil,
			}},
		}
		return nil
	}
	apiCaller := apitesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "InstanceMutater")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchModelMachines")
			c.Check(a, gc.IsNil)
			return nil
		},
	)
	facadeCaller.ReturnRawAPICaller = apitesting.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}

	api := instancemutater.NewClient(apiCaller)
	_, err := api.WatchModelMachines()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterSuite) TestWatchModelMachinesServerError(c *gc.C) {
	apiCaller := clientErrorAPICaller(c, "WatchModelMachines", nil)
	api := instancemutater.NewClient(apiCaller)
	w, err := api.WatchModelMachines()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(w, gc.IsNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
}

func (s *instanceMutaterSuite) TestMachineCallsLife(c *gc.C) {
	// We have tested separately the Life method, here we just check
	// it's called internally.
	expectedResults := params.LifeResults{
		Results: []params.LifeResult{{Life: "working"}},
	}
	entitiesArgs := params.Entities{
		Entities: []params.Entity{
			{Tag: s.tag.String()},
		},
	}
	apiCaller := successAPICaller(c, "Life", entitiesArgs, expectedResults)
	api := instancemutater.NewClient(apiCaller)
	m, err := api.Machine(names.NewMachineTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiCaller.CallCount, gc.Equals, 1)
	c.Assert(m.Tag().String(), gc.Equals, s.tag.String())
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstanceMutater",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Error:         errors.New("client error!"),
	})
}

func successAPICaller(c *gc.C, method string, expectArgs, useResults interface{}) *apitesting.CallChecker {
	return apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        "InstanceMutater",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	})
}
