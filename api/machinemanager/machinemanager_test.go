// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"errors"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachinemanagerSuite{})

type MachinemanagerSuite struct {
	coretesting.BaseSuite
}

func (s *MachinemanagerSuite) TestAddMachines(c *gc.C) {
	apiResult := []params.AddMachinesResult{
		{Machine: "machine-1", Error: nil},
		{Machine: "machine-2", Error: nil},
	}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
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

	st := machinemanager.NewClient(apiCaller)
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
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := machinemanager.NewClient(apiCaller)
	_, err := st.AddMachines(nil)
	c.Check(err, gc.ErrorMatches, "blargh")
}

func (s *MachinemanagerSuite) TestAddMachinesServerError(c *gc.C) {
	apiResult := []params.AddMachinesResult{{
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.AddMachinesResults)) = params.AddMachinesResults{
			Machines: apiResult,
		}
		return nil
	})
	st := machinemanager.NewClient(apiCaller)
	machines := []params.AddMachineParams{{
		Series: "trusty",
	}}
	results, err := st.AddMachines(machines)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, apiResult)
}

func (s *MachinemanagerSuite) TestAddMachinesResultCountInvalid(c *gc.C) {
	for _, n := range []int{0, 2} {
		apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			var results []params.AddMachinesResult
			for i := 0; i < n; i++ {
				results = append(results, params.AddMachinesResult{
					Error: &params.Error{Message: "MSG", Code: "621"},
				})
			}
			*(result.(*params.AddMachinesResults)) = params.AddMachinesResults{Machines: results}
			return nil
		})
		st := machinemanager.NewClient(apiCaller)
		machines := []params.AddMachineParams{{
			Series: "trusty",
		}}
		_, err := st.AddMachines(machines)
		c.Check(err, gc.ErrorMatches, fmt.Sprintf("expected 1 result, got %d", n))
	}
}
