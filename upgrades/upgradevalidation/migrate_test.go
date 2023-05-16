// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
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
	"23.04",
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
	ctrl, cloudSpec := s.setupJuju3Target(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	targetVersion := version.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion, cloudSpec)

	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), s.statePool, s.st, s.model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju31(c *gc.C) {
	ctrl, cloudSpec := s.setupJuju3Target(c)
	defer ctrl.Finish()

	// - check no charm store charms
	s.st.EXPECT().AllCharmURLs().Return([]*string{}, errors.NotFoundf("charm urls"))

	modelTag := coretesting.ModelTag
	targetVersion := version.MustParse("3.1.0")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion, cloudSpec)

	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), s.statePool, s.st, s.model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju2(c *gc.C) {
	defer s.initializeMocks(c).Finish()

	modelTag := coretesting.ModelTag

	// - check agent version;
	s.model.EXPECT().AgentVersion().Return(version.MustParse("2.9.32"), nil)
	// - check no upgrade series in process.
	s.st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil)

	targetVersion := version.MustParse("2.9.99")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion, environscloudspec.CloudSpec{Type: "foo"})
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

func (s *migrateSuite) setupJuju3Target(c *gc.C) (*gomock.Controller, environscloudspec.CloudSpec) {
	ctrl := s.initializeMocks(c)
	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)
	// - check LXD version.
	cloudSpec := environscloudspec.CloudSpec{Type: "lxd"}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	// - check agent version;
	s.model.EXPECT().AgentVersion().Return(version.MustParse("2.9.43"), nil)
	// - check no upgrade series in process.
	s.st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil)
	// - check if the model has win machines;
	s.st.EXPECT().MachineCountForBase(makeBases("windows", winVersions)).Return(nil, nil)
	s.st.EXPECT().MachineCountForBase(makeBases("ubuntu", ubuntuVersions)).Return(nil, nil)

	return ctrl, cloudSpec
}
