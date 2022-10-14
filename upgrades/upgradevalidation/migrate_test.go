// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"net/http"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades/upgradevalidation"
	"github.com/juju/juju/upgrades/upgradevalidation/mocks"
)

var winVersions = []string{
	"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
	"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
}

var ubuntuVersions = []string{
	"12.04",
	"12.10",
	"13.04",
	"13.10",
	"14.04",
	"14.10",
	"15.04",
	"15.10",
	"16.04",
	"16.10",
	"17.04",
	"17.10",
	"18.04",
	"18.10",
	"19.04",
	"19.10",
	"20.10",
	"21.04",
	"21.10",
	"22.10",
}

func makeBases(os string, vers []string) []state.Base {
	bases := make([]state.Base, len(vers))
	for i, vers := range vers {
		bases[i] = state.Base{OS: os, Channel: vers}
	}
	return bases
}

func (s *upgradeValidationSuite) TestValidatorsForModelMigrationSourceJuju3(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(httpClient *http.Client) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := environscloudspec.CloudSpec{Type: "lxd"}

	gomock.InOrder(
		// - check agent version;
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.32"), nil),
		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
		// - check if the model has win machines;
		state.EXPECT().MachineCountForBase(makeBases("windows", winVersions)).Return(nil, nil),
		state.EXPECT().MachineCountForBase(makeBases("ubuntu", ubuntuVersions)).Return(nil, nil),
		// - check LXD version.
		serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil),
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	targetVersion := version.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion, cloudSpec)
	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), statePool, state, model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *upgradeValidationSuite) TestValidatorsForModelMigrationSourceJuju2(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	gomock.InOrder(
		// - check agent version;
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.32"), nil),
		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
	)

	targetVersion := version.MustParse("2.9.99")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion, environscloudspec.CloudSpec{Type: "foo"})
	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), statePool, state, model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}
