// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/diskformatter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/names"
)

var _ = gc.Suite(&DiskFormatterSuite{})

type DiskFormatterSuite struct {
	coretesting.BaseSuite
}

func (s *DiskFormatterSuite) TestBlockDevices(c *gc.C) {
	devices := []params.BlockDeviceResult{{
		Result: storage.BlockDevice{DeviceName: "sda", Size: 123},
	}, {
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "DiskFormatter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "BlockDevices")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "disk-0"}, {Tag: "disk-1"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.BlockDeviceResults{})
		*(result.(*params.BlockDeviceResults)) = params.BlockDeviceResults{
			devices,
		}
		called = true
		return nil
	})

	st := diskformatter.NewState(apiCaller, names.NewUnitTag("service/0"))
	results, err := st.BlockDevices([]names.DiskTag{
		names.NewDiskTag("0"),
		names.NewDiskTag("1"),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(results.Results, gc.DeepEquals, devices)
}

func (s *DiskFormatterSuite) TestBlockDeviceResultCountMismatch(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.BlockDeviceResults)) = params.BlockDeviceResults{
			[]params.BlockDeviceResult{{}, {}},
		}
		return nil
	})
	st := diskformatter.NewState(apiCaller, names.NewUnitTag("service/0"))
	c.Assert(func() { st.BlockDevices(nil) }, gc.PanicMatches, "expected 0 results, got 2")
}

func (s *DiskFormatterSuite) TestBlockDeviceStorageInstances(c *gc.C) {
	storageInstances := []params.StorageInstanceResult{{
		Result: storage.StorageInstance{Id: "whatever"},
	}, {
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	var called bool
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "DiskFormatter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "BlockDeviceStorageInstances")
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "disk-0"}, {Tag: "disk-1"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StorageInstanceResults{})
		*(result.(*params.StorageInstanceResults)) = params.StorageInstanceResults{
			storageInstances,
		}
		called = true
		return nil
	})

	st := diskformatter.NewState(apiCaller, names.NewUnitTag("service/0"))
	results, err := st.BlockDeviceStorageInstances([]names.DiskTag{
		names.NewDiskTag("0"),
		names.NewDiskTag("1"),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Assert(results.Results, gc.DeepEquals, storageInstances)
}

func (s *DiskFormatterSuite) TestAPIErrors(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("blargh")
	})
	st := diskformatter.NewState(apiCaller, names.NewUnitTag("service/0"))
	_, err := st.BlockDevices(nil)
	c.Check(err, gc.ErrorMatches, "blargh")
	_, err = st.BlockDeviceStorageInstances(nil)
	c.Check(err, gc.ErrorMatches, "blargh")
}
