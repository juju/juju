// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

type apiaddresserSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&apiaddresserSuite{})

func (s *apiaddresserSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *apiaddresserSuite) TestAPIAddresses(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := apimocks.NewMockFacadeCaller(ctrl)
	result := params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	}
	facade.EXPECT().FacadeCall(gomock.Any(), "APIAddresses", nil, gomock.Any()).SetArg(3, result).Return(nil)

	client := common.NewAPIAddresser(facade)
	addresses, err := client.APIAddresses(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, []string{"0.1.2.3:1234"})
}

func (s *apiaddresserSuite) TestAPIHostPorts(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	facade := apimocks.NewMockFacadeCaller(ctrl)

	ports := []corenetwork.MachineHostPorts{
		{{
			MachineAddress: corenetwork.NewMachineAddress("1.2.3.4", corenetwork.WithScope(corenetwork.ScopePublic)),
			NetPort:        1234,
		}, {
			MachineAddress: corenetwork.NewMachineAddress("2.3.4.5", corenetwork.WithScope(corenetwork.ScopePublic)),
			NetPort:        2345,
		}}, {{
			MachineAddress: corenetwork.NewMachineAddress("3.4.5.6", corenetwork.WithScope(corenetwork.ScopePublic)),
			NetPort:        3456,
		}},
	}

	hps := make([]corenetwork.HostPorts, len(ports))
	for i, mHP := range ports {
		hps[i] = mHP.HostPorts()
	}
	result := params.APIHostPortsResult{
		Servers: params.FromHostsPorts(hps),
	}

	facade.EXPECT().FacadeCall(gomock.Any(), "APIHostPorts", nil, gomock.Any()).SetArg(3, result).Return(nil)

	client := common.NewAPIAddresser(facade)

	expectServerAddrs := []corenetwork.ProviderHostPorts{
		{
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("1.2.3.4").AsProviderAddress(), NetPort: 1234},
			corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("2.3.4.5").AsProviderAddress(), NetPort: 2345},
		},
		{corenetwork.ProviderHostPort{ProviderAddress: corenetwork.NewMachineAddress("3.4.5.6").AsProviderAddress(), NetPort: 3456}},
	}

	serverAddrs, err := client.APIHostPorts(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverAddrs, tc.DeepEquals, expectServerAddrs)
}

func (s *apiaddresserSuite) TestWatchAPIHostPorts(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	facade := apimocks.NewMockFacadeCaller(ctrl)
	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("NotifyWatcher").Return(666)
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Next", nil, gomock.Any()).Return(nil).AnyTimes()
	caller.EXPECT().APICall(gomock.Any(), "NotifyWatcher", 666, "", "Stop", nil, gomock.Any()).Return(nil).AnyTimes()

	result := params.NotifyWatchResult{}
	facade.EXPECT().FacadeCall(gomock.Any(), "WatchAPIHostPorts", nil, gomock.Any()).SetArg(3, result).Return(nil)
	facade.EXPECT().RawAPICaller().Return(caller)

	client := common.NewAPIAddresser(facade)
	w, err := client.WatchAPIHostPorts(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-w.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}
