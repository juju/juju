// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/mocks"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/feature"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type statusSuite struct {
	baseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.ControllerModel(c).State().AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

// Complete testing of status functionality happens elsewhere in the codebase,
// these tests just sanity-check the api itself.

func (s *statusSuite) TestFullStatus(c *gc.C) {
	machine := s.addMachine(c)
	st := s.ControllerModel(c).State()
	c.Assert(st.SetSLA("essential", "test-user", []byte("")), jc.ErrorIsNil)
	c.Assert(st.SetModelMeterStatus("GREEN", "goo"), jc.ErrorIsNil)
	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.Name, gc.Equals, "controller")
	c.Check(status.Model.Type, gc.Equals, "iaas")
	c.Check(status.Model.CloudTag, gc.Equals, "cloud-dummy")
	c.Check(status.Model.SLA, gc.Equals, "essential")
	c.Check(status.Model.MeterStatus.Color, gc.Equals, "green")
	c.Check(status.Model.MeterStatus.Message, gc.Equals, "goo")
	c.Check(status.Applications, gc.HasLen, 0)
	c.Check(status.RemoteApplications, gc.HasLen, 0)
	c.Check(status.Offers, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	c.Check(status.ControllerTimestamp, gc.NotNil)
	c.Check(status.Branches, gc.HasLen, 0)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Base, jc.DeepEquals, params.Base{Name: "ubuntu", Channel: "12.10/stable"})
	c.Check(resultMachine.LXDProfiles, gc.HasLen, 0)
}

func (s *statusSuite) TestUnsupportedNoModelMeterStatus(c *gc.C) {
	s.addMachine(c)
	st := s.ControllerModel(c).State()
	c.Assert(st.SetSLA("unsupported", "test-user", []byte("")), jc.ErrorIsNil)
	c.Assert(st.SetModelMeterStatus("RED", "nope"), jc.ErrorIsNil)
	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.SLA, gc.Equals, "unsupported")
	c.Check(status.Model.MeterStatus.Color, gc.Equals, "")
	c.Check(status.Model.MeterStatus.Message, gc.Equals, "")
}

func (s *statusSuite) TestFullStatusUnitLeadership(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	u := f.MakeUnit(c, nil)
	st := s.ControllerModel(c).State()
	claimer, err := s.LeaseManager.Claimer("application-leadership", st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(u.ApplicationName(), u.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	app, ok := status.Applications[u.ApplicationName()]
	c.Assert(ok, jc.IsTrue)
	unit, ok := app.Units[u.Name()]
	c.Assert(ok, jc.IsTrue)
	c.Assert(unit.Leader, jc.IsTrue)
}

func (s *statusSuite) TestFullStatusUnitScaling(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	machine := f.MakeMachine(c, nil)
	unit := f.MakeUnit(c, &factory.UnitParams{
		Machine: machine,
	})
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add several more units of the same application to the
	// same machine. We do this because we want to isolate to
	// status handling to just additional units, not additional machines
	// or applications.
	app, err := unit.Application()
	c.Assert(err, jc.ErrorIsNil)

	const numUnits = 5
	for i := 0; i < numUnits; i++ {
		f.MakeUnit(c, &factory.UnitParams{
			Application: app,
			Machine:     machine,
		})
	}

	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount+numUnits*3,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of units, please fix it"))
}

func (s *statusSuite) TestFullStatusMachineScaling(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	f.MakeMachine(c, nil)
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add several more machines to the model.
	for i := 0; i < 5; i++ {
		f.MakeMachine(c, nil)
	}
	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of machines, please fix it"))
}

func (s *statusSuite) TestFullStatusInterfaceScaling(c *gc.C) {
	machine := s.addMachine(c)
	s.createSpaceAndSubnetWithProviderID(c, "public", "10.0.0.0/24", "prov-0000")
	s.createSpaceAndSubnetWithProviderID(c, "private", "10.20.0.0/24", "prov-ffff")
	s.createSpaceAndSubnetWithProviderID(c, "dmz", "10.30.0.0/24", "prov-abcd")
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add a bunch of interfaces to the machine.
	s.createNICWithIP(c, machine, "eth0", "10.0.0.11/24")
	s.createNICWithIP(c, machine, "eth1", "10.20.0.42/24")
	s.createNICWithIP(c, machine, "eth2", "10.30.0.99/24")

	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth1",
			ConfigMethod:      network.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-ffff",
			CIDRAddress:       "10.20.0.42/24",
		},
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth2",
			ConfigMethod:      network.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-abcd",
			CIDRAddress:       "10.30.0.99/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the way the addresses are processed"))
}

func (s *statusSuite) createSpaceAndSubnetWithProviderID(c *gc.C, spaceName, CIDR, providerSubnetID string) {
	st := s.ControllerModel(c).State()
	space, err := st.AddSpace(spaceName, network.Id(spaceName), nil, true)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.AddSubnet(network.SubnetInfo{
		CIDR:       CIDR,
		SpaceID:    space.Id(),
		ProviderId: network.Id(providerSubnetID),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusSuite) createNICWithIP(c *gc.C, machine *state.Machine, deviceName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       network.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: network.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	host := f.MakeMachine(c, &factory.MachineParams{InstanceId: "0"})
	container := f.MakeMachineNested(c, host.Id(), nil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 1)

	_, ok = mStatus.Containers[container.Id()]
	c.Check(ok, jc.IsTrue)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	host := f.MakeMachine(c, &factory.MachineParams{InstanceId: "1"})
	f.MakeMachineNested(c, host.Id(), nil)
	lxdHost := f.MakeMachineNested(c, host.Id(), nil)
	f.MakeMachineNested(c, lxdHost.Id(), nil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 2)

	mStatus, ok = mStatus.Containers[lxdHost.Id()]
	c.Check(ok, jc.IsTrue)

	c.Check(mStatus.Containers, gc.HasLen, 1)
}

var testUnits = []struct {
	unitName       string
	setStatus      *state.MeterStatus
	expectedStatus *params.MeterStatus
}{{
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterAmber, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "amber", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterRed, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "red", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {},
}

func (s *statusUnitTestSuite) TestModelMeterStatus(c *gc.C) {
	st := s.ControllerModel(c).State()
	c.Assert(st.SetSLA("advanced", "test-user", nil), jc.ErrorIsNil)
	c.Assert(st.SetModelMeterStatus("RED", "thing"), jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	modelMeterStatus := status.Model.MeterStatus
	c.Assert(modelMeterStatus.Color, gc.Equals, "red")
	c.Assert(modelMeterStatus.Message, gc.Equals, "thing")
}

func (s *statusUnitTestSuite) TestMeterStatus(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := app.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	appStatus, ok := status.Applications[app.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(appStatus.MeterStatuses, gc.HasLen, len(testUnits)-1)
	for _, unit := range testUnits {
		unitStatus, ok := appStatus.MeterStatuses[unit.unitName]

		if unit.expectedStatus != nil {
			c.Assert(ok, gc.Equals, true)
			c.Assert(&unitStatus, gc.DeepEquals, unit.expectedStatus)
		} else {
			c.Assert(ok, gc.Equals, false)
		}
	}
}

func (s *statusUnitTestSuite) TestNoMeterStatusWhenNotRequired(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	app := f.MakeApplication(c, nil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := app.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	appStatus, ok := status.Applications[app.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(appStatus.MeterStatuses, gc.HasLen, 0)
}

func (s *statusUnitTestSuite) TestMeterStatusWithCredentials(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	app := f.MakeApplication(c, nil)
	c.Assert(app.SetMetricCredentials([]byte("magic-ticket")), jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := app.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	appStatus, ok := status.Applications[app.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(appStatus.MeterStatuses, gc.HasLen, len(testUnits)-1)
	for _, unit := range testUnits {
		unitStatus, ok := appStatus.MeterStatuses[unit.unitName]

		if unit.expectedStatus != nil {
			c.Assert(ok, gc.Equals, true)
			c.Assert(&unitStatus, gc.DeepEquals, unit.expectedStatus)
		} else {
			c.Assert(ok, gc.Equals, false)
		}
	}
}

func (s *statusUnitTestSuite) TestApplicationWithExposedEndpoints(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	appStatus, ok := status.Applications[app.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(appStatus.ExposedEndpoints, gc.DeepEquals, map[string]params.ExposedEndpoint{
		"": {
			ExposeToSpaces: []string{network.AlphaSpaceName},
			ExposeToCIDRs:  []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	})
}

func (s *statusUnitTestSuite) TestPrincipalUpgradingFrom(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	meteredCharm := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered-3"})
	meteredCharmNew := f.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "ch:amd64/quantal/metered-5"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	u := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
		SetCharmURL: true,
	})
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok := status.Applications[app.Name()].Units[u.Name()]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "")

	err = app.SetCharm(state.SetCharmConfig{
		Charm: meteredCharmNew,
	})
	c.Assert(err, jc.ErrorIsNil)

	status, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok = status.Applications[app.Name()].Units[u.Name()]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "ch:amd64/quantal/metered-3")
}

func (s *statusUnitTestSuite) TestSubordinateUpgradingFrom(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	principalCharm := f.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "ch:amd64/quantal/mysql"})
	subordCharm := f.MakeCharm(c, &factory.CharmParams{Name: "logging", URL: "ch:amd64/quantal/logging-1"})
	subordCharmNew := f.MakeCharm(c, &factory.CharmParams{Name: "logging", URL: "ch:amd64/quantal/logging-2"})
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: principalCharm,
		Name:  "principal",
	})
	pu := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	subordApp := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: subordCharm,
		Name:  "subord",
	})

	subEndpoint, err := subordApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	principalEndpoint, err := app.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.AddRelation(subEndpoint, principalEndpoint)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(pu)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subordUnit, err := st.Unit("subord/0")
	c.Assert(err, jc.ErrorIsNil)
	err = subordUnit.SetCharmURL(subordCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok := status.Applications["principal"].Units["principal/0"].Subordinates["subord/0"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "")

	err = subordApp.SetCharm(state.SetCharmConfig{
		Charm: subordCharmNew,
	})
	c.Assert(err, jc.ErrorIsNil)

	status, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok = status.Applications["principal"].Units["principal/0"].Subordinates["subord/0"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "ch:amd64/quantal/logging-1")
}

func addUnitWithVersion(c *gc.C, application *state.Application, version string) *state.Unit {
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the timestamp on this version record is different
	// from the previous one.
	// TODO(babbageclunk): when Application and Unit have clocks, change
	// that instead of sleeping (lp:1558657)
	time.Sleep(time.Millisecond * 1)
	err = unit.SetWorkloadVersion(version)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *statusUnitTestSuite) checkAppVersion(c *gc.C, application *state.Application, expectedVersion string,
) params.ApplicationStatus {
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	appStatus, found := status.Applications[application.Name()]
	c.Assert(found, jc.IsTrue)
	c.Check(appStatus.WorkloadVersion, gc.Equals, expectedVersion)
	return appStatus
}

func checkUnitVersion(c *gc.C, appStatus params.ApplicationStatus, unit *state.Unit, expectedVersion string) {
	unitStatus, found := appStatus.Units[unit.Name()]
	c.Check(found, jc.IsTrue)
	c.Check(unitStatus.WorkloadVersion, gc.Equals, expectedVersion)
}

func (s *statusUnitTestSuite) TestWorkloadVersionLastWins(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	application := f.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")
	unit2 := addUnitWithVersion(c, application, "voltron")
	unit3 := addUnitWithVersion(c, application, "zarkon")

	appStatus := s.checkAppVersion(c, application, "zarkon")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "voltron")
	checkUnitVersion(c, appStatus, unit3, "zarkon")
}

func (s *statusUnitTestSuite) TestWorkloadVersionSimple(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	application := f.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")

	appStatus := s.checkAppVersion(c, application, "voltron")
	checkUnitVersion(c, appStatus, unit1, "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionBlanksCanWin(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	application := f.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")
	unit2 := addUnitWithVersion(c, application, "")

	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionNoUnits(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	application := f.MakeApplication(c, nil)
	s.checkAppVersion(c, application, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionOkWithUnset(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	application := f.MakeApplication(c, nil)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit, "")
}

func (s *statusUnitTestSuite) TestMigrationInProgress(c *gc.C) {
	st := s.ControllerModel(c).State()
	setGenerationsControllerConfig(c, st)
	// Create a host model because controller models can't be migrated.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	state2 := f.MakeModel(c, nil)
	defer state2.Close()

	model2, err := state2.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Get API connection to hosted model.
	apiInfo := s.ModelApiInfo(model2.UUID())
	apiInfo.Tag = testing.AdminUser
	apiInfo.Password = testing.AdminSecret

	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	checkMigStatus := func(expected string) {
		status, err := client.Status(nil)
		c.Assert(err, jc.ErrorIsNil)
		if expected != "" {
			expected = "migrating: " + expected
		}
		c.Check(status.Model.ModelStatus.Info, gc.Equals, expected)
	}

	// Migration status should be empty when no migration is happening.
	checkMigStatus("")

	// Start it migrating.
	mig, err := state2.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check initial message.
	checkMigStatus("starting")

	// Check status is reported when set.
	setAndCheckMigStatus := func(message string) {
		err := mig.SetStatusMessage(message)
		c.Assert(err, jc.ErrorIsNil)
		checkMigStatus(message)
	}
	setAndCheckMigStatus("proceeding swimmingly")
	setAndCheckMigStatus("oh noes")
}

func (s *statusUnitTestSuite) TestRelationFiltered(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// make application 1 with endpoint 1
	a1 := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "abc",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := a1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	// make application 2 with endpoint 2
	a2 := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "def",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := a2.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between a1 and a2
	r12 := f.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(r12, gc.NotNil)

	// create another application 3 with an endpoint 3
	a3 := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	e3, err := a3.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	// create endpoint 4 on application 1
	e4, err := a1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	r13 := f.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e3, e4},
	})
	c.Assert(r13, gc.NotNil)

	// Test status filtering with application 1: should get both relations
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status([]string{a1.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a1.Name(), 2, status.Relations)

	// test status filtering with application 3: should get 1 relation
	status, err = client.Status([]string{a3.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a3.Name(), 1, status.Relations)
}

// TestApplicationFilterIndependentOfAlphabeticUnitOrdering ensures we
// do not regress and are carrying forward fix for lp#1592872.
func (s *statusUnitTestSuite) TestApplicationFilterIndependentOfAlphabeticUnitOrdering(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
		Name: "abc",
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
		Name: "def",
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := f.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	for i := 0; i < 20; i++ {
		c.Logf("run %d", i)
		status, err := client.Status([]string{applicationA.Name()})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Applications, gc.HasLen, 2)
	}
}

// TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly
// tests scenario where applications are returned as part of the status because
// they are related to an application that matches given filter.
// However, the relations for these applications should not be returned.
// In other words, if there are two applications, A and B, such that:
//
// * an application A matches the supplied filter directly;
// * an application B has units on the same machine as units of an application A and, thus,
// qualifies to be returned by the status result;
//
// application B's relations should not be returned.
func (s *statusUnitTestSuite) TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	f.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := f.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	// Filtering status on application A should get:
	// * no relations;
	// * two applications.
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status([]string{applicationA.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Assert(status.Applications, gc.HasLen, 2)
	c.Assert(status.Relations, gc.HasLen, 0)
}

func (s *statusUnitTestSuite) TestMachineWithNoDisplayNameHasItsEmptyDisplayNameSent(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	machine := f.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("i-123"),
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "")
}

func (s *statusUnitTestSuite) TestMachineWithDisplayNameHasItsDisplayNameSent(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	machine := f.MakeMachine(c, &factory.MachineParams{
		InstanceId:  instance.Id("i-123"),
		DisplayName: "snowflake",
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "snowflake")
}

func assertApplicationRelations(c *gc.C, appName string, expectedNumber int, relations []params.RelationStatus) {
	c.Assert(relations, gc.HasLen, expectedNumber)
	for _, relation := range relations {
		belongs := false
		for _, endpoint := range relation.Endpoints {
			if endpoint.ApplicationName == appName {
				belongs = true
				continue
			}
		}
		if !belongs {
			c.Fatalf("application %v is not part of the relation %v as expected", appName, relation.Id)
		}
	}
}

type statusUpgradeUnitSuite struct {
	testing.ApiServerSuite

	charms               map[string]*state.Charm
	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	ctrl                 *gomock.Controller
}

var _ = gc.Suite(&statusUpgradeUnitSuite{})

func (s *statusUpgradeUnitSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithLeaseManager = true
	s.ApiServerSuite.SetUpTest(c)
	s.charms = make(map[string]*state.Charm)

	state := charmrevisionupdater.StateShim{State: s.ControllerModel(c).State()}
	s.ctrl = gomock.NewController(c)
	charmhubClient := mocks.NewMockCharmhubRefreshClient(s.ctrl)
	charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]transport.RefreshResponse{
		{Entity: transport.RefreshEntity{Revision: 42}},
	}, nil)
	newCharmhubClient := func(st charmrevisionupdater.State) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return charmhubClient, nil
	}

	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, clock.WallClock,
		newCharmhubClient, loggo.GetLogger("juju.apiserver.charmrevisionupdater"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusUpgradeUnitSuite) TearDownTest(c *gc.C) {
	s.ApiServerSuite.TearDownTest(c)
	s.ctrl.Finish()
}

// AddMachine adds a new machine to state.
func (s *statusUpgradeUnitSuite) AddMachine(c *gc.C, machineId string, job state.MachineJob) {
	m, err := s.ControllerModel(c).State().AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, machineId)
}

// AddCharmhubCharmWithRevision adds a charmhub charm with the specified revision to state.
func (s *statusUpgradeUnitSuite) AddCharmhubCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := testcharms.Hub.CharmDir(charmName)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("ch:amd64/jammy/%s-%d", name, rev))
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := s.ControllerModel(c).State().AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
	return dummy
}

// AddApplication adds an application for the specified charm to state.
func (s *statusUpgradeUnitSuite) AddApplication(c *gc.C, charmName, applicationName string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	revision := ch.Revision()
	_, err := s.ControllerModel(c).State().AddApplication(state.AddApplicationArgs{
		Name:  applicationName,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			ID:     "mycharmhubid",
			Hash:   "mycharmhash",
			Source: "charm-hub",
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "12.10/stable",
			},
			Revision: &revision,
			Channel: &state.Channel{
				Track: "latest",
				Risk:  "stable",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

// AddUnit adds a new unit for application to the specified machine.
func (s *statusUpgradeUnitSuite) AddUnit(c *gc.C, appName, machineId string) {
	app, err := s.ControllerModel(c).State().Application(appName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.ControllerModel(c).State().Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}

// SetUnitRevision sets the unit's charm to the specified revision.
func (s *statusUpgradeUnitSuite) SetUnitRevision(c *gc.C, unitName string, rev int) {
	u, err := s.ControllerModel(c).State().Unit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := u.Application()
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL(fmt.Sprintf("ch:amd64/jammy/%s-%d", svc.Name(), rev))
	err = u.SetCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
}

// SetupScenario adds some machines and applications to state.
// It assumes a controller machine has already been created.
func (s *statusUpgradeUnitSuite) SetupScenario(c *gc.C) {
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddMachine(c, "2", state.JobHostUnits)
	s.AddMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.AddCharmhubCharmWithRevision(c, "mysql", 22)
	s.AddApplication(c, "mysql", "mysql")
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmhubCharmWithRevision(c, "wordpress", 26)
	s.AddApplication(c, "wordpress", "wordpress")
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmhubCharmWithRevision(c, "varnish", 5)
	s.AddApplication(c, "varnish", "varnish")
	s.AddUnit(c, "varnish", "3")
}

func (s *statusUpgradeUnitSuite) TestUpdateRevisionsCharmhub(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddCharmhubCharmWithRevision(c, "charmhubby", 41)
	s.AddApplication(c, "charmhubby", "charmhubby")
	s.AddUnit(c, "charmhubby", "1")

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	status, _ := client.Status(nil)

	appStatus, ok := status.Applications["charmhubby"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(appStatus.CanUpgradeTo, gc.Equals, "")

	// Update to the latest available charm revision.
	result, err := s.charmrevisionupdater.UpdateLatestRevisions(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Check if CanUpgradeTo suggests the latest revision.
	status, _ = client.Status(nil)
	appStatus, ok = status.Applications["charmhubby"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(appStatus.CanUpgradeTo, gc.Equals, "ch:amd64/jammy/charmhubby-42")
}

type CAASStatusSuite struct {
	baseSuite

	model *state.Model
	app   *state.Application
}

var _ = gc.Suite(&CAASStatusSuite{})

func (s *CAASStatusSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// Set up a CAAS model to replace the IAAS one.
	st := f.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.model = m

	ch := f2.MakeCharm(c, &factory.CharmParams{
		Name:   "mysql-k8s",
		Series: "focal",
	})
	s.app = f2.MakeApplication(c, &factory.ApplicationParams{
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm: ch,
	})
	f2.MakeUnit(c, &factory.UnitParams{Application: s.app})
}

func (s *CAASStatusSuite) TestStatusOperatorNotReady(c *gc.C) {
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "waiting", "installing agent")
}

func (s *CAASStatusSuite) TestStatusCloudContainerSet(c *gc.C) {
	loggo.GetLogger("juju.state.status").SetLogLevel(loggo.TRACE)
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	u, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		u[0].UpdateOperation(state.UnitUpdateProperties{
			CloudContainerStatus: &status.StatusInfo{Status: status.Blocked, Message: "blocked"},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "blocked", "blocked")
}

func (s *CAASStatusSuite) assertUnitStatus(c *gc.C, appStatus params.ApplicationStatus, status, info string) {
	curl, _ := s.app.CharmURL()
	workloadVersion := ""
	if info != "installing agent" && info != "blocked" {
		workloadVersion = "gitlab/latest"
	}
	c.Assert(appStatus, jc.DeepEquals, params.ApplicationStatus{
		Charm:           *curl,
		Base:            params.Base{Name: "ubuntu", Channel: "20.04/stable"},
		WorkloadVersion: workloadVersion,
		Relations:       map[string][]string{},
		SubordinateTo:   []string{},
		Units: map[string]params.UnitStatus{
			s.app.Name() + "/0": {
				AgentStatus: params.DetailedStatus{
					Status: "allocating",
				},
				WorkloadStatus: params.DetailedStatus{
					Status: status,
					Info:   info,
				},
			},
		},
		Status: params.DetailedStatus{
			Status: status,
			Info:   info,
		},
		EndpointBindings: map[string]string{
			"":             network.AlphaSpaceName,
			"server":       network.AlphaSpaceName,
			"server-admin": network.AlphaSpaceName,
		},
	})
}

func (s *CAASStatusSuite) TestStatusWorkloadVersionSetByCharm(c *gc.C) {
	loggo.GetLogger("juju.state.allwatcher").SetLogLevel(loggo.TRACE)
	conn := s.OpenModelAPI(c, s.model.UUID())
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.SetScale(1, 1, true)
	c.Assert(err, jc.ErrorIsNil)
	u, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u, gc.HasLen, 1)
	err = u[0].SetWorkloadVersion("666")
	c.Assert(err, jc.ErrorIsNil)
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	app := status.Applications[s.app.Name()]
	c.Assert(app.WorkloadVersion, gc.Equals, "666")
	c.Assert(app.Scale, gc.Equals, 1)
}

type filteringBranchesSuite struct {
	baseSuite

	appA string
	appB string
	subB string
}

var _ = gc.Suite(&filteringBranchesSuite{})

func (s *filteringBranchesSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithLeaseManager = true
	s.baseSuite.SetUpTest(c)
	setGenerationsControllerConfig(c, s.ControllerModel(c).State())

	s.appA = "mysql"
	s.appB = "wordpress"
	s.subB = "logging"

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: s.appA,
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: s.appB,
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	f.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
	})
	appBUnit := f.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
	})

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := f.MakeApplication(c, &factory.ApplicationParams{
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: s.subB}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel := f.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})
	// Trigger the creation of the subordinate unit by entering scope
	// on the principal unit.
	ru, err := rel.Unit(appBUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *filteringBranchesSuite) TestFullStatusBranchNoFilter(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.AddBranch("apple", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", status.Branches)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{})
	c.Assert(status.Applications, gc.HasLen, 3)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.appA+"/0")
	st := s.ControllerModel(c).State()
	err := st.AddBranch("banana", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status([]string{s.appA + "/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appA: {s.appA + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 1)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterUnitLeader(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.appA+"/0")
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.ControllerModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(s.appA, s.appA+"/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	st := s.ControllerModel(c).State()
	err = st.AddBranch("banana", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status([]string{s.appA + "/leader"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appA: {s.appA + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 1)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterApplication(c *gc.C) {
	st := s.ControllerModel(c).State()
	err := st.AddBranch("apple", "test-user")
	c.Assert(err, jc.ErrorIsNil)
	s.assertBranchAssignApplication(c, "banana", s.appB)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status([]string{s.appB})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["banana"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appB: {}})
	c.Assert(status.Applications, gc.HasLen, 2)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterSubordinateUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.subB+"/0")
	s.assertBranchAssignUnit(c, "banana", s.appA+"/0")
	st := s.ControllerModel(c).State()
	err := st.AddBranch("cucumber", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status([]string{s.subB + "/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.subB: {s.subB + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 2)

}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterTwoBranchesSubordinateUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.subB+"/0")
	s.assertBranchAssignUnit(c, "banana", s.appA+"/0")
	s.assertBranchAssignUnit(c, "cucumber", s.appB+"/0")

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, coretesting.NoopLogger{})

	status, err := client.Status([]string{s.appB + "/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 2)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.subB: {s.subB + "/0"}})
	b, ok = status.Branches["cucumber"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appB: {s.appB + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 2)
}

func (s *filteringBranchesSuite) assertBranchAssignUnit(c *gc.C, bName, uName string) {
	st := s.ControllerModel(c).State()
	err := st.AddBranch(bName, "test-user")
	c.Assert(err, jc.ErrorIsNil)
	gen, err := st.Branch(bName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)
	err = gen.AssignUnit(uName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *filteringBranchesSuite) assertBranchAssignApplication(c *gc.C, bName, aName string) {
	st := s.ControllerModel(c).State()
	err := st.AddBranch(bName, "test-user")
	c.Assert(err, jc.ErrorIsNil)
	gen, err := st.Branch(bName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)
	err = gen.AssignApplication(aName)
	c.Assert(err, jc.ErrorIsNil)
}

func setGenerationsControllerConfig(c *gc.C, st *state.State) {
	err := st.UpdateControllerConfig(map[string]interface{}{
		"features": feature.Branches,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}
