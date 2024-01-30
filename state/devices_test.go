// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

	if s.series == "focal" {
		s.st = s.Factory.MakeCAASModel(c, nil)
		s.AddCleanup(func(_ *gc.C) { s.st.Close() })
	} else {
		s.st = s.State
		s.series = "quantal"
	}
	var err error
	s.deviceBackend, err = state.NewDeviceBackend(s.st)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DevicesStateSuiteBase) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharmForSeries(c, s.st, s.series, name)
}

func makeDeviceCons(t state.DeviceType, count int64) state.DeviceConstraints {
	return state.DeviceConstraints{Type: t, Count: count}
}

type CAASDevicesStateSuite struct {
	DevicesStateSuiteBase
}

func (s *CAASDevicesStateSuite) SetUpTest(c *gc.C) {
	// Use focal for k8s charms (quantal for machine charms).
	s.series = "focal"
	s.DevicesStateSuiteBase.SetUpTest(c)
}

var _ = gc.Suite(&CAASDevicesStateSuite{})

func (s *CAASDevicesStateSuite) TestAddApplicationDevicesConstraintsValidation(c *gc.C) {
	ch := s.AddTestingCharm(c, "bitcoin-miner")
	addApplication := func(devices map[string]state.DeviceConstraints) (*state.Application, error) {
		return s.st.AddApplication(state.AddApplicationArgs{Name: "bitcoin-miner", Charm: ch,
			Devices: devices,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "20.04/stable",
			}},
		}, mockApplicationSaver{}, state.NewObjectStore(c, s.st.ModelUUID()))
	}
	assertErr := func(devices map[string]state.DeviceConstraints, expect string) {
		_, err := addApplication(devices)
		c.Assert(err, gc.ErrorMatches, expect)
	}

	deviceCons := map[string]state.DeviceConstraints{
		"bitcoinminer-incorrect-name": makeDeviceCons("nvidia.com/gpu", 2),
	}
	assertErr(deviceCons, `cannot add application "bitcoin-miner": charm "bitcoin-miner" has no device called "bitcoinminer-incorrect-name"`)
	deviceCons = map[string]state.DeviceConstraints{
		"bitcoinminer": makeDeviceCons("nvidia.com/gpu", 0),
	}
	assertErr(deviceCons, `cannot add application "bitcoin-miner": charm "bitcoin-miner" device "bitcoinminer": minimum device size is 1, 0 specified`)
	deviceCons["bitcoinminer"] = makeDeviceCons("nvidia.com/gpu", 2)
	app, err := addApplication(deviceCons)
	c.Assert(err, jc.ErrorIsNil)

	var devs map[string]state.DeviceConstraints
	devs, err = app.DeviceConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devs, jc.DeepEquals, map[string]state.DeviceConstraints{
		"bitcoinminer": {
			Type:       "nvidia.com/gpu",
			Count:      2,
			Attributes: map[string]string{},
		},
	})
}
