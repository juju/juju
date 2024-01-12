// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

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
	"23.04",
	"23.10",
	"24.04",
}

func makeBases(os string, vers []string) []state.Base {
	bases := make([]state.Base, len(vers))
	for i, vers := range vers {
		bases[i] = state.Base{OS: os, Channel: vers}
	}
	return bases
}

var _ = gc.Suite(&migrateSuite{})

type migrateSuite struct {
	jujutesting.IsolationSuite

	st        *mocks.MockState
	statePool *mocks.MockStatePool
	model     *mocks.MockModel
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju3(c *gc.C) {
	ctrl, cloudSpec := s.setupMocks(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	validators := upgradevalidation.ValidatorsForModelMigrationSource(cloudSpec)

	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), s.statePool, s.st, s.model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju31(c *gc.C) {
	ctrl, cloudSpec := s.setupMocks(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	validators := upgradevalidation.ValidatorsForModelMigrationSource(cloudSpec)

	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), s.statePool, s.st, s.model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) initializeMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.statePool = mocks.NewMockStatePool(ctrl)
	s.st = mocks.NewMockState(ctrl)
	s.model = mocks.NewMockModel(ctrl)
	return ctrl
}

func (s *migrateSuite) setupMocks(c *gc.C) (*gomock.Controller, environscloudspec.CloudSpec) {
	ctrl := s.initializeMocks(c)
	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)
	// - check LXD version.
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	// - check no upgrade series in process.
	s.st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil)
	// - check if the model has win machines;
	s.st.EXPECT().MachineCountForBase(makeBases("ubuntu", ubuntuVersions)).Return(nil, nil)

	return ctrl, cloudSpec.CloudSpec
}
