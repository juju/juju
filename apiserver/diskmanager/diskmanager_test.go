// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/diskmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DiskManagerSuite{})

type DiskManagerSuite struct {
	coretesting.BaseSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	api        *diskmanager.DiskManagerAPI
}

func (s *DiskManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	tag := names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	s.st = &mockState{}
	diskmanager.PatchState(s, s.st)

	var err error
	s.api, err = diskmanager.NewDiskManagerAPI(nil, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevices(c *gc.C) {
	devices := []storage.BlockDevice{{DeviceName: "sda"}, {DeviceName: "sdb"}}
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine:      "machine-0",
			BlockDevices: devices,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesEmptyArgs(c *gc.C) {
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *DiskManagerSuite) TestNewDiskManagerAPINonMachine(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := diskmanager.NewDiskManagerAPI(nil, nil, s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesInvalidTags(c *gc.C) {
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}, {
			Machine: "machine-1",
		}, {
			Machine: "unit-mysql-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: &params.Error{"permission denied", "unauthorized access"},
		}, {
			Error: &params.Error{"permission denied", "unauthorized access"},
		}},
	})
	c.Assert(s.st.calls, gc.Equals, 1)
}

func (s *DiskManagerSuite) TestSetMachineBlockDevicesStateError(c *gc.C) {
	s.st.err = errors.New("boom")
	results, err := s.api.SetMachineBlockDevices(params.SetMachineBlockDevices{
		MachineBlockDevices: []params.MachineBlockDevices{{
			Machine: "machine-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{"boom", ""},
		}},
	})
}

type mockState struct {
	calls   int
	devices map[string][]state.BlockDeviceInfo
	err     error
}

func (st *mockState) SetMachineBlockDevices(machineId string, devices []state.BlockDeviceInfo) error {
	st.calls++
	if st.devices == nil {
		st.devices = make(map[string][]state.BlockDeviceInfo)
	}
	st.devices[machineId] = devices
	return st.err
}
