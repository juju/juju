// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&MachinemanagerSuite{})

type MachinemanagerSuite struct {
}

func (s *MachinemanagerSuite) TestAddMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiResult := []params.AddMachinesResult{
		{Machine: "machine-1", Error: nil},
		{Machine: "machine-2", Error: nil},
	}

	machines := []params.AddMachineParams{{
		Base:  &params.Base{Name: "ubuntu", Channel: "22.04"},
		Disks: []storage.Directive{{Pool: "loop", Size: 1}},
	}, {
		Base: &params.Base{Name: "ubuntu", Channel: "20.04"},
	}}

	args := params.AddMachines{
		MachineParams: machines,
	}
	res := new(params.AddMachinesResults)
	results := params.AddMachinesResults{
		Machines: apiResult,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddMachines", args, res).SetArg(3, results).Return(nil)
	st := machinemanager.NewClientFromCaller(mockFacadeCaller)

	result, err := st.AddMachines(context.Background(), machines)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, apiResult)
}

func (s *MachinemanagerSuite) TestAddMachinesClientError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.AddMachines{
		MachineParams: []params.AddMachineParams{{}},
	}
	res := new(params.AddMachinesResults)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	st := machinemanager.NewClientFromCaller(mockFacadeCaller)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddMachines", args, res).Return(errors.New("blargh"))
	_, err := st.AddMachines(context.Background(), []params.AddMachineParams{{}})
	c.Check(err, tc.ErrorMatches, "blargh")
}

func (s *MachinemanagerSuite) TestAddMachinesServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiResult := []params.AddMachinesResult{{
		Error: &params.Error{Message: "MSG", Code: "621"},
	}}

	machines := []params.AddMachineParams{{
		Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
	}}
	args := params.AddMachines{
		MachineParams: machines,
	}
	res := new(params.AddMachinesResults)
	ress := params.AddMachinesResults{
		Machines: apiResult,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddMachines", args, res).SetArg(3, ress).Return(nil)
	st := machinemanager.NewClientFromCaller(mockFacadeCaller)
	results, err := st.AddMachines(context.Background(), machines)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, apiResult)
}

func (s *MachinemanagerSuite) TestAddMachinesResultCountInvalid(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	for _, n := range []int{0, 2} {
		machines := []params.AddMachineParams{{
			Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
		}}
		args := params.AddMachines{
			MachineParams: machines,
		}
		res := new(params.AddMachinesResults)
		var results []params.AddMachinesResult
		for i := 0; i < n; i++ {
			results = append(results, params.AddMachinesResult{
				Error: &params.Error{Message: "MSG", Code: "621"},
			})
		}
		ress := params.AddMachinesResults{
			Machines: results,
		}
		mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
		mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddMachines", args, res).SetArg(3, ress).Return(nil)
		st := machinemanager.NewClientFromCaller(mockFacadeCaller)
		_, err := st.AddMachines(context.Background(), machines)
		c.Check(err, tc.ErrorMatches, fmt.Sprintf("expected 1 result, got %d", n))
	}
}

func (s *MachinemanagerSuite) TestRetryProvisioning(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RetryProvisioningArgs{
		All: false,
		Machines: []string{
			names.NewMachineTag("0").String(),
			names.NewMachineTag("1").String()},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{Results: []params.ErrorResult{
		{Error: &params.Error{Code: "boom"}},
		{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RetryProvisioning", args, res).SetArg(3, ress).Return(nil)
	client := machinemanager.NewClientFromCaller(mockFacadeCaller)
	result, err := client.RetryProvisioning(context.Background(), false, names.NewMachineTag("0"), names.NewMachineTag("1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Code: "boom"}},
		{},
	})
}

func (s *MachinemanagerSuite) TestRetryProvisioningAll(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.RetryProvisioningArgs{
		All:      true,
		Machines: []string{},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{Results: []params.ErrorResult{
		{Error: &params.Error{Code: "boom"}},
		{}},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RetryProvisioning", args, res).SetArg(3, ress).Return(nil)
	client := machinemanager.NewClientFromCaller(mockFacadeCaller)
	result, err := client.RetryProvisioning(context.Background(), true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []params.ErrorResult{
		{Error: &params.Error{Code: "boom"}},
		{},
	})
}

func (s *MachinemanagerSuite) TestProvisioningScript(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ProvisioningScriptParams{
		MachineId:              "0",
		Nonce:                  "nonce",
		DataDir:                "/path/to/data",
		DisablePackageCommands: true,
	}
	res := new(params.ProvisioningScriptResult)
	ress := params.ProvisioningScriptResult{Script: "script"}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ProvisioningScript", args, res).SetArg(3, ress).Return(nil)
	client := machinemanager.NewClientFromCaller(mockFacadeCaller)

	script, err := client.ProvisioningScript(context.Background(), params.ProvisioningScriptParams{
		MachineId:              "0",
		Nonce:                  "nonce",
		DataDir:                "/path/to/data",
		DisablePackageCommands: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(script, tc.Equals, "script")
}

func (s *MachinemanagerSuite) clientToTestDestroyMachinesWithParams(maxWait *time.Duration, ctrl *gomock.Controller) (*machinemanager.Client, []params.DestroyMachineResult) {
	expectedResults := []params.DestroyMachineResult{{
		Error: &params.Error{Message: "boo"},
	}, {
		Info: &params.DestroyMachineInfo{
			DestroyedUnits:   []params.Entity{{Tag: "unit-foo-0"}},
			DestroyedStorage: []params.Entity{{Tag: "storage-pgdata-0"}},
			DetachedStorage:  []params.Entity{{Tag: "storage-pgdata-1"}},
		},
	}}

	args := params.DestroyMachinesParams{
		Keep:  true,
		Force: true,
		MachineTags: []string{
			"machine-0",
			"machine-0-lxd-1",
		},
		MaxWait: maxWait,
	}
	res := new(params.DestroyMachineResults)
	ress := params.DestroyMachineResults{Results: expectedResults}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DestroyMachineWithParams", args, res).SetArg(3, ress).Return(nil)
	client := machinemanager.NewClientFromCaller(mockFacadeCaller)

	return client, expectedResults
}

func (s *MachinemanagerSuite) TestDestroyMachinesWithParamsNoWait(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	noWait := 0 * time.Second
	client, expected := s.clientToTestDestroyMachinesWithParams(&noWait, ctrl)
	results, err := client.DestroyMachinesWithParams(context.Background(), true, true, false, &noWait, "0", "0/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expected)
}

func (s *MachinemanagerSuite) TestDestroyMachinesWithParamsNilWait(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	client, expected := s.clientToTestDestroyMachinesWithParams((*time.Duration)(nil), ctrl)
	results, err := client.DestroyMachinesWithParams(context.Background(), true, true, false, (*time.Duration)(nil), "0", "0/lxd/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, expected)
}
