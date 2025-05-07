// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	"context"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/highavailability"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/rpc/params"
)

type clientSuite struct {
}

var _ = tc.Suite(&clientSuite{})

func (s *clientSuite) TestClientEnableHA(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	emptyCons := constraints.Value{}

	args := params.ControllersSpecs{Specs: []params.ControllersSpec{{
		Constraints:    emptyCons,
		NumControllers: 3,
		Placement:      nil,
	},
	}}
	res := new(params.ControllersChangeResults)
	results := params.ControllersChangeResults{
		Results: []params.ControllersChangeResult{{
			Result: params.ControllersChanges{
				Maintained: []string{"machine-0"},
				Added:      []string{"machine-1", "machine-2"},
				Removed:    []string{},
			}},
		}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "EnableHA", args, res).SetArg(3, results).Return(nil)
	mockClient := basemocks.NewMockClientFacade(ctrl)
	client := highavailability.NewClientFromCaller(mockFacadeCaller, mockClient)

	result, err := client.EnableHA(context.Background(), 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(result.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(result.Removed, tc.HasLen, 0)
}

func (s *clientSuite) TestControllerDetails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	res := new(params.ControllerDetailsResults)
	results := params.ControllerDetailsResults{
		Results: []params.ControllerDetails{{
			ControllerId: "666",
			APIAddresses: []string{"address"},
		}}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ControllerDetails", nil, res).SetArg(3, results).Return(nil)
	mockClient := basemocks.NewMockClientFacade(ctrl)
	mockClient.EXPECT().BestAPIVersion().Return(3)
	client := highavailability.NewClientFromCaller(mockFacadeCaller, mockClient)

	result, err := client.ControllerDetails(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, map[string]highavailability.ControllerDetails{
		"666": {
			ControllerID: "666",
			APIEndpoints: []string{"address"},
		},
	})
}
