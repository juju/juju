// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mutex/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/container/broker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type fakePrepareAPI struct {
	*testhelpers.Stub
	requestedBridges []network.DeviceToBridge
}

var _ broker.PrepareAPI = (*fakePrepareAPI)(nil)

func (api *fakePrepareAPI) HostChangesForContainer(ctx context.Context, tag names.MachineTag) ([]network.DeviceToBridge, error) {
	api.Stub.MethodCall(api, "HostChangesForContainer", tag)
	if err := api.Stub.NextErr(); err != nil {
		return nil, err
	}
	return api.requestedBridges, nil
}

func (api *fakePrepareAPI) SetHostMachineNetworkConfig(ctx context.Context, tag names.MachineTag, config []params.NetworkConfig) error {
	api.Stub.MethodCall(api, "SetHostMachineNetworkConfig", tag, config)
	if err := api.Stub.NextErr(); err != nil {
		return err
	}
	return nil
}

type hostPreparerSuite struct {
	Stub *testhelpers.Stub
}

func TestHostPreparerSuite(t *stdtesting.T) {
	tc.Run(t, &hostPreparerSuite{})
}

func (s *hostPreparerSuite) SetUpTest(c *tc.C) {
	s.Stub = &testhelpers.Stub{}
}

type stubReleaser struct {
	*testhelpers.Stub
}

func (r *stubReleaser) Release() {
	r.MethodCall(r, "Release")
}

func (s *hostPreparerSuite) acquireStubLock(_ string, _ <-chan struct{}) (func(), error) {
	s.Stub.AddCall("AcquireLock")
	if err := s.Stub.NextErr(); err != nil {
		return nil, err
	}
	releaser := &stubReleaser{
		Stub: s.Stub,
	}
	return releaser.Release, nil
}

type stubBridger struct {
	*testhelpers.Stub
}

var _ network.Bridger = (*stubBridger)(nil)

func (br *stubBridger) Bridge(devices []network.DeviceToBridge) error {
	br.Stub.MethodCall(br, "Bridge", devices)
	if err := br.Stub.NextErr(); err != nil {
		return err
	}
	return nil
}

type cannedNetworkObserver struct {
	*testhelpers.Stub
	config []params.NetworkConfig
}

func (cno *cannedNetworkObserver) ObserveNetwork() ([]params.NetworkConfig, error) {
	cno.Stub.AddCall("ObserveNetwork")
	if err := cno.Stub.NextErr(); err != nil {
		return nil, err
	}
	return cno.config, nil
}

func (s *hostPreparerSuite) createPreparerParams(c *tc.C, bridges []network.DeviceToBridge, observed []params.NetworkConfig) broker.HostPreparerParams {
	observer := &cannedNetworkObserver{
		Stub:   s.Stub,
		config: observed,
	}
	return broker.HostPreparerParams{
		API: &fakePrepareAPI{
			Stub:             s.Stub,
			requestedBridges: bridges,
		},
		AcquireLockFunc:    s.acquireStubLock,
		Bridger:            &stubBridger{s.Stub},
		ObserveNetworkFunc: observer.ObserveNetwork,
		MachineTag:         names.NewMachineTag("1"),
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *hostPreparerSuite) createPreparer(c *tc.C, bridges []network.DeviceToBridge, observed []params.NetworkConfig) *broker.HostPreparer {
	params := s.createPreparerParams(c, bridges, observed)
	return broker.NewHostPreparer(params)
}

func (s *hostPreparerSuite) TestPrepareHostNoChanges(c *tc.C) {
	preparer := s.createPreparer(c, nil, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Assert(err, tc.ErrorIsNil)
	// If HostChangesForContainer returns nothing to change, then we don't
	// instantiate a Bridger, or do any bridging.
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		},
		{
			FuncName: "Release",
		},
	})
}

var cannedObservedNetworkConfig = []params.NetworkConfig{{
	DeviceIndex:         0,
	MACAddress:          "aa:bb:cc:dd:ee:ff",
	MTU:                 1500,
	InterfaceName:       "lo",
	ParentInterfaceName: "",
	InterfaceType:       string(corenetwork.LoopbackDevice),
	Disabled:            false,
	NoAutoStart:         false,
	ConfigType:          string(corenetwork.ConfigLoopback),
}, {
	DeviceIndex:         1,
	MACAddress:          "bb:cc:dd:ee:ff:00",
	MTU:                 1500,
	InterfaceName:       "eth0",
	ParentInterfaceName: "br-eth0",
	InterfaceType:       string(corenetwork.EthernetDevice),
	Disabled:            false,
	NoAutoStart:         false,
	ConfigType:          string(corenetwork.ConfigStatic),
}, {
	DeviceIndex:         2,
	MACAddress:          "bb:cc:dd:ee:ff:00",
	MTU:                 1500,
	InterfaceName:       "br-eth0",
	ParentInterfaceName: "",
	InterfaceType:       string(corenetwork.BridgeDevice),
	Disabled:            false,
	NoAutoStart:         false,
	ConfigType:          string(corenetwork.ConfigStatic),
}}

func (s *hostPreparerSuite) TestPrepareHostCreateBridge(c *tc.C) {
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, cannedObservedNetworkConfig)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Assert(err, tc.ErrorIsNil)
	// This should be the normal flow if there are changes necessary. We read
	// the changes, grab a bridger, then acquire a lock, do the bridging,
	// observe the results, report the results, and release the lock.
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{devices},
		}, {
			FuncName: "ObserveNetwork",
		}, {
			FuncName: "SetHostMachineNetworkConfig",
			Args:     []interface{}{names.NewMachineTag("1"), cannedObservedNetworkConfig},
		}, {
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostNothingObserved(c *tc.C) {
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	observed := []params.NetworkConfig(nil)
	preparer := s.createPreparer(c, devices, observed)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Assert(err, tc.ErrorIsNil)
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{devices},
		}, {
			FuncName: "ObserveNetwork",
			// We don't call SetHostMachineNetworkConfig if ObserveNetwork returns nothing
		}, {
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostChangesUnsupported(c *tc.C) {
	// ensure that errors calling HostChangesForContainer are treated as
	// provisioning errors, instead of assuming we can continue creating a
	// container.
	s.Stub.SetErrors(
		nil,
		errors.NotSupportedf("container address allocation"),
		nil,
	)
	preparer := s.createPreparer(c, nil, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Assert(err, tc.ErrorMatches, "unable to setup network: container address allocation not supported")
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostNoBridger(c *tc.C) {
	s.Stub.SetErrors(
		nil, // AcquireLock
		nil, // HostChangesForContainer
		errors.New("unable to find python interpreter"), // Bridge
		nil, //Release
	)
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, ".*unable to find python interpreter")

	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{[]network.DeviceToBridge{{DeviceName: "eth0", BridgeName: "br-eth0", MACAddress: ""}}},
		}, {
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostNoLock(c *tc.C) {
	s.Stub.SetErrors(
		mutex.ErrTimeout, // AcquireLock
	)
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, `failed to acquire machine lock for bridging: timeout acquiring mutex`)

	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostBridgeFailure(c *tc.C) {
	s.Stub.SetErrors(
		nil, // HostChangesForContainer
		nil, // AcquireLock
		errors.New("script invocation error: IOError"), // Bridge
	)
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, `failed to bridge devices: script invocation error: IOError`)
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{devices},
		}, {
			// We don't observe the network information.
			// TODO(jam): 2017-02-15 This is possibly wrong, we might consider
			// a) Forcibly restarting if Bridge() fails,
			// b) Still observing and reporting our observation so that we at least
			//    know what state we ended up in.
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostObserveFailure(c *tc.C) {
	s.Stub.SetErrors(
		nil, // HostChangesForContainer
		nil, // AcquireLock
		nil, // BridgeBridgeFailure
		errors.New("cannot get network interfaces: enoent"), // GetObservedNetworkConfig
	)
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, nil)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, `cannot discover observed network config: cannot get network interfaces: enoent`)
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{devices},
		}, {
			FuncName: "ObserveNetwork",
		}, {
			// We don't SetHostMachineNetworkConfig, but we still release the lock.
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostObservedFailure(c *tc.C) {
	s.Stub.SetErrors(
		nil,                             // HostChangesForContainer
		nil,                             // AcquireLock
		nil,                             // BridgeBridgeFailure
		nil,                             // ObserveNetwork
		errors.Unauthorizedf("failure"), // SetHostMachineNetworkConfig
	)
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	preparer := s.createPreparer(c, devices, cannedObservedNetworkConfig)
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, `failure`)
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLock",
		}, {
			FuncName: "HostChangesForContainer",
			Args:     []interface{}{containerTag},
		}, {
			FuncName: "Bridge",
			Args:     []interface{}{devices},
		}, {
			FuncName: "ObserveNetwork",
		}, {
			FuncName: "SetHostMachineNetworkConfig",
			Args:     []interface{}{names.NewMachineTag("1"), cannedObservedNetworkConfig},
		}, {
			FuncName: "Release",
		},
	})
}

func (s *hostPreparerSuite) TestPrepareHostCancel(c *tc.C) {
	devices := []network.DeviceToBridge{{
		DeviceName: "eth0",
		BridgeName: "br-eth0",
	}}
	args := s.createPreparerParams(c, devices, nil)
	ch := make(chan struct{})
	close(ch)
	args.AbortChan = ch
	// This is what the AcquireLock should look like.
	args.AcquireLockFunc = func(_ string, abort <-chan struct{}) (func(), error) {
		s.Stub.AddCall("AcquireLockFunc")
		// Make sure that the right channel got passed in.
		c.Check(abort, tc.Equals, (<-chan struct{})(ch))
		select {
		case <-abort:
			return nil, errors.Errorf("AcquireLock cancelled")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timeout waiting for channel to return aborted")
			return nil, errors.Errorf("timeout triggered")
		}
	}
	preparer := broker.NewHostPreparer(args)
	// Now when we prepare, we should fail with "cancelled".
	containerTag := names.NewMachineTag("1/lxd/0")
	err := preparer.Prepare(c.Context(), containerTag)
	c.Check(err, tc.ErrorMatches, `failed to acquire machine lock for bridging: AcquireLock cancelled`)
	s.Stub.CheckCalls(c, []testhelpers.StubCall{
		{
			FuncName: "AcquireLockFunc",
		},
	})
}
