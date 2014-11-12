// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"errors"
	"fmt"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/diskmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

var _ = gc.Suite(&DiskManagerSuite{})

type DiskManagerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *gc.C) {
	devices := []storage.BlockDevice{{
		DeviceName: "sda",
		Size:       123,
	}, {
		DeviceName: "sdb",
		UUID:       "asdadasdasdas",
	}}

	var called int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "DiskManager")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetMachineBlockDevices")
		c.Assert(arg, gc.DeepEquals, params.SetMachineBlockDevices{
			MachineBlockDevices: []params.MachineBlockDevices{{
				Machine:      "machine-123",
				BlockDevices: devices,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		called++
		return nil
	})

	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(devices)
	c.Assert(err, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesNil(c *gc.C) {
	var called int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(arg, gc.DeepEquals, params.SetMachineBlockDevices{
			MachineBlockDevices: []params.MachineBlockDevices{{
				Machine: "machine-123",
			}},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		called++
		return nil
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(called, gc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesClientError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(nil)
	c.Assert(err, gc.ErrorMatches, "blargh")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesServerError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(nil)
	c.Assert(err, gc.ErrorMatches, "MSG")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesResultCountInvalid(c *gc.C) {
	for _, n := range []int{0, 2} {
		apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			var results []params.ErrorResult
			for i := 0; i < n; i++ {
				results = append(results, params.ErrorResult{
					Error: &params.Error{Message: "MSG", Code: "621"},
				})
			}
			*(result.(*params.ErrorResults)) = params.ErrorResults{Results: results}
			return nil
		})
		st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
		err := st.SetMachineBlockDevices(nil)
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("expected 1 result, got %d", n))
	}
}
