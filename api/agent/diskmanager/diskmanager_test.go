// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/diskmanager"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/blockdevice"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&DiskManagerSuite{})

type DiskManagerSuite struct {
	coretesting.BaseSuite
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *tc.C) {
	devices := []blockdevice.BlockDevice{{
		DeviceName: "sda",
		SizeMiB:    123,
	}, {
		DeviceName: "sdb",
		UUID:       "asdadasdasdas",
	}}

	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "DiskManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetMachineBlockDevices")
		c.Check(arg, tc.DeepEquals, params.SetMachineBlockDevices{
			MachineBlockDevices: []params.MachineBlockDevices{{
				Machine: "machine-123",
				BlockDevices: []params.BlockDevice{{
					DeviceName: "sda",
					Size:       123,
				}, {
					DeviceName: "sdb",
					UUID:       "asdadasdasdas",
				}},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		callCount++
		return nil
	})

	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(context.Background(), devices)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesNil(c *tc.C) {
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(arg, tc.DeepEquals, params.SetMachineBlockDevices{
			MachineBlockDevices: []params.MachineBlockDevices{{
				Machine: "machine-123",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		callCount++
		return nil
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(context.Background(), nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesClientError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(context.Background(), nil)
	c.Check(err, tc.ErrorMatches, "blargh")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesServerError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "MSG", Code: "621"},
			}},
		}
		return nil
	})
	st := diskmanager.NewState(apiCaller, names.NewMachineTag("123"))
	err := st.SetMachineBlockDevices(context.Background(), nil)
	c.Check(err, tc.ErrorMatches, "MSG")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesResultCountInvalid(c *tc.C) {
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
		err := st.SetMachineBlockDevices(context.Background(), nil)
		c.Check(err, tc.ErrorMatches, fmt.Sprintf("expected 1 result, got %d", n))
	}
}
