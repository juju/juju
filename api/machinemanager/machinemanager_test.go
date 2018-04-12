// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"errors"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachinemanagerSuite{})

type MachinemanagerSuite struct {
	coretesting.BaseSuite
}

func newClient(f basetesting.APICallerFunc) *machinemanager.Client {
	return machinemanager.NewClient(f)
}

func (s *MachinemanagerSuite) TestAddMachines(c *gc.C) {
	apiResult := []params.AddMachinesResult{
		{Machine: "machine-1", Error: nil},
		{Machine: "machine-2", Error: nil},
	}

	var callCount int
	st := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "MachineManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "AddMachines")
		c.Check(arg, gc.DeepEquals, params.AddMachines{
			MachineParams: []params.AddMachineParams{
				{
					Series: "trusty",
					Disks:  []storage.Constraints{{Pool: "loop", Size: 1}},
				},
				{
					Series: "precise",
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.AddMachinesResults{})
		*(result.(*params.AddMachinesResults)) = params.AddMachinesResults{
			Machines: apiResult,
		}
		callCount++
		return nil
	})

	machines := []params.AddMachineParams{{
		Series: "trusty",
		Disks:  []storage.Constraints{{Pool: "loop", Size: 1}},
	}, {
		Series: "precise",
	}}
	result, err := st.AddMachines(machines)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, apiResult)
	c.Check(callCount, gc.Equals, 1)
}

func (s *MachinemanagerSuite) TestAddMachinesClientError(c *gc.C) {
	st := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	_, err := st.AddMachines(nil)
	c.Check(err, gc.ErrorMatches, "blargh")
}

func (s *MachinemanagerSuite) TestAddMachinesServerError(c *gc.C) {
	apiResult := []params.AddMachinesResult{{
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	st := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.AddMachinesResults)) = params.AddMachinesResults{
			Machines: apiResult,
		}
		return nil
	})
	machines := []params.AddMachineParams{{
		Series: "trusty",
	}}
	results, err := st.AddMachines(machines)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, apiResult)
}

func (s *MachinemanagerSuite) TestAddMachinesResultCountInvalid(c *gc.C) {
	for _, n := range []int{0, 2} {
		st := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
			var results []params.AddMachinesResult
			for i := 0; i < n; i++ {
				results = append(results, params.AddMachinesResult{
					Error: &params.Error{Message: "MSG", Code: "621"},
				})
			}
			*(result.(*params.AddMachinesResults)) = params.AddMachinesResults{Machines: results}
			return nil
		})
		machines := []params.AddMachineParams{{
			Series: "trusty",
		}}
		_, err := st.AddMachines(machines)
		c.Check(err, gc.ErrorMatches, fmt.Sprintf("expected 1 result, got %d", n))
	}
}

func (s *MachinemanagerSuite) TestDestroyMachines(c *gc.C) {
	s.testDestroyMachines(c, "DestroyMachine", (*machinemanager.Client).DestroyMachines)
}

func (s *MachinemanagerSuite) TestForceDestroyMachines(c *gc.C) {
	s.testDestroyMachines(c, "ForceDestroyMachine", (*machinemanager.Client).ForceDestroyMachines)
}

func (s *MachinemanagerSuite) testDestroyMachines(
	c *gc.C,
	methodName string,
	method func(*machinemanager.Client, ...string) ([]params.DestroyMachineResult, error),
) {
	expectedResults := []params.DestroyMachineResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyMachineInfo{
			DestroyedUnits:   []params.Entity{{Tag: "unit-foo-0"}},
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, methodName)
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{
				{Tag: "machine-0"},
				{Tag: "machine-0-lxd-1"},
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyMachineResults{})
		out := response.(*params.DestroyMachineResults)
		*out = params.DestroyMachineResults{expectedResults}
		return nil
	})
	results, err := method(client, "0", "0/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *MachinemanagerSuite) TestDestroyMachinesArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		return nil
	})
	_, err := client.DestroyMachines("0")
	c.Assert(err, gc.ErrorMatches, `expected 1 result\(s\), got 0`)
}

func (s *MachinemanagerSuite) TestDestroyMachinesInvalidIds(c *gc.C) {
	expectedResults := []params.DestroyMachineResult{{
		Error: &params.Error{Message: `machine ID "!" not valid`},
	}, {
		Info: &params.DestroyMachineInfo{},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		out := response.(*params.DestroyMachineResults)
		*out = params.DestroyMachineResults{expectedResults[1:]}
		return nil
	})
	results, err := client.DestroyMachines("!", "0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *MachinemanagerSuite) TestDestroyMachinesWithParams(c *gc.C) {
	expectedResults := []params.DestroyMachineResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyMachineInfo{
			DestroyedUnits:   []params.Entity{{Tag: "unit-foo-0"}},
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}
	client := newClient(func(objType string, version int, id, request string, a, response interface{}) error {
		c.Assert(request, gc.Equals, "DestroyMachineWithParams")
		c.Assert(a, jc.DeepEquals, params.DestroyMachinesParams{
			Keep:  true,
			Force: true,
			MachineTags: []string{
				"machine-0",
				"machine-0-lxd-1",
			},
		})
		c.Assert(response, gc.FitsTypeOf, &params.DestroyMachineResults{})
		out := response.(*params.DestroyMachineResults)
		*out = params.DestroyMachineResults{expectedResults}
		return nil
	})
	results, err := client.DestroyMachinesWithParams(true, true, "0", "0/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expectedResults)
}
