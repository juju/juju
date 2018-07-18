// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	mocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgradeseries"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
)

type upgradeSeriesSuite struct {
	testing.IsolationSuite
	testing.Stub

	resources  *common.Resources
	st         *stateShim
	unitTag    names.UnitTag
	machineTag names.MachineTag

	ctrl        *gomock.Controller
	mockBackend *mocks.MockUpgradeSeriesBackend
	mockMachine *mocks.MockUpgradeSeriesMachine
	mockUnit    *mocks.MockUpgradeSeriesUnit
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpSuite(c *gc.C) {
	s.machineTag = names.NewMachineTag("0")
	s.resources = common.NewResources()
	s.unitTag = names.NewUnitTag("testing/0")
}

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	s.ctrl = gomock.NewController(c)
	s.mockBackend = mocks.NewMockUpgradeSeriesBackend(s.ctrl)
	//s.st = &stateShim{s.mockBackend}
}

func (s *upgradeSeriesSuite) newMachineUpgradeSeriesAPI(c *gc.C) *upgradeseries.UpgradeSeriesAPI {
	auth := apiservertesting.FakeAuthorizer{Tag: s.machineTag}
	return s.newUpgradeSeriesAPI(c, auth)
}

func (s *upgradeSeriesSuite) newUnitUpgradeSeriesAPI(c *gc.C) *upgradeseries.UpgradeSeriesAPI {
	auth := apiservertesting.FakeAuthorizer{Tag: s.unitTag}
	return s.newUpgradeSeriesAPI(c, auth)
}

func (s *upgradeSeriesSuite) newUpgradeSeriesAPI(c *gc.C, auth apiservertesting.FakeAuthorizer) *upgradeseries.UpgradeSeriesAPI {
	api, err := upgradeseries.NewUpgradeSeriesAPI(stateShim{s.mockBackend}, s.resources, auth)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

type stateShim struct {
	*mocks.MockUpgradeSeriesBackend
}

func (m *stateShim) Machine(id string) (state.Machine, error) {
	return m.Machine(id)
}

func (m *stateShim) Unit(id string) (state.Unit, error) {
	return m.Unit(id)
}
