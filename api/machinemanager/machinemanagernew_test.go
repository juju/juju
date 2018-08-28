// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&NewMachineManagerSuite{})

type NewMachineManagerSuite struct {
	jujutesting.BaseSuite

	tag  names.Tag
	args params.Entities
}

func (s *NewMachineManagerSuite) SetUpTest(c *gc.C) {

	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}

	s.BaseSuite.SetUpTest(c)
}

func (s *NewMachineManagerSuite) TestUnitsToUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	fFacade := mocks.NewMockClientFacade(ctrl)
	fCaller := mocks.NewMockFacadeCaller(ctrl)
	arbitraryName := "machine-0"

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{
			{
				Entity: params.Entity{Tag: names.NewMachineTag(arbitraryName).String()},
			},
		},
	}
	result := params.UpgradeSeriesUnitsResult{
		UnitNames: []string{"ubuntu/0", "ubuntu/1"},
	}

	results := params.UpgradeSeriesUnitsResults{[]params.UpgradeSeriesUnitsResult{result}}

	fFacade.EXPECT().BestAPIVersion().Return(5)
	fCaller.EXPECT().FacadeCall("UnitsToUpgrade", args, gomock.Any()).SetArg(2, results)
	client := machinemanager.MakeClient(fFacade, fCaller)

	unitNames, err := client.UnitsToUpgrade(arbitraryName)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(unitNames, gc.DeepEquals, result.UnitNames)
}
