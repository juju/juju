// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/state"
)

type DevicesStateSuiteBase struct {
	ConnSuite

	series        string
	st            *state.State
	deviceBackend *state.DeviceBackend
}

func (s *DevicesStateSuiteBase) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error

	if s.series == "kubernetes" {
		s.st = s.Factory.MakeCAASModel(c, nil)
		s.AddCleanup(func(_ *gc.C) { s.st.Close() })
	} else {
		s.st = s.State
		s.series = "quantal"
	}
	s.deviceBackend, err = state.NewDeviceBackend(s.st)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DevicesStateSuiteBase) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharmForSeries(c, s.st, s.series, name)
}

func makeDeviceCons(t devices.DeviceType, count int64) devices.Constraints {
	return devices.Constraints{Type: t, Count: count}
}

type DevicesStateSuite struct {
	DevicesStateSuiteBase
}

var _ = gc.Suite(&DevicesStateSuite{})

func (s *DevicesStateSuite) TestAddApplicationDevicesConstraintsValidation(c *gc.C) {
	ch := s.AddTestingCharm(c, "bitcoin-miner")
	addApplication := func(devices map[string]devices.Constraints) (*state.Application, error) {
		return s.st.AddApplication(state.AddApplicationArgs{Name: "bitcoin-miner", Charm: ch, Devices: devices})
	}
	assertErr := func(devices map[string]devices.Constraints, expect string) {
		_, err := addApplication(devices)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	deviceCons := map[string]devices.Constraints{
		"bitcoinminer-incorrect-name": makeDeviceCons("nvidia.com/gpu", 2),
	}
	assertErr(deviceCons, `charm bitcoinminer has no device called bitcoinminer-incorrect-name`)
	deviceCons = map[string]devices.Constraints{
		"bitcoinminer": makeDeviceCons("nvidia.com/gpu", 0),
	}
	assertErr(deviceCons, `minimum device size is 1, 0 specified`)
	deviceCons["bitcoinminer"] = makeDeviceCons("nvidia.com/gpu", 2)
	_, err := addApplication(deviceCons)
	c.Assert(err, jc.ErrorIsNil)
}
