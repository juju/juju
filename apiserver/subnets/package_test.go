// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"fmt"
	"net"
	stdtesting "testing"

	"github.com/juju/testing"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/subnets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	StubProviderType               = "stub-provider"
	StubEnvironName                = "stub-environ"
	StubZonedEnvironName           = "stub-zoned-environ"
	StubNetworkingEnvironName      = "stub-networking-environ"
	StubZonedNetworkingEnvironName = "stub-zoned-networking-environ"
)

var (
	// SharedStub records all method calls to any of the stubs.
	SharedStub = &testing.Stub{}

	BackingInstance                = &StubBacking{Stub: SharedStub}
	ProviderInstance               = &StubProvider{Stub: SharedStub}
	EnvironInstance                = &StubEnviron{Stub: SharedStub}
	ZonedEnvironInstance           = &StubZonedEnviron{Stub: SharedStub}
	NetworkingEnvironInstance      = &StubNetworkingEnviron{Stub: SharedStub}
	ZonedNetworkingEnvironInstance = &StubZonedNetworkingEnviron{Stub: SharedStub}
)

func init() {
	ProviderInstance.Zones = []providercommon.AvailabilityZone{
		&FakeZone{"zone1", true},
		&FakeZone{"zone2", false},
		&FakeZone{"zone3", true},
		&FakeZone{"zone4", false},
		&FakeZone{"zone4", false}, // duplicates are ignored
	}
	ProviderInstance.Subnets = []network.SubnetInfo{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-zadf00d",
		AvailabilityZones: []string{"zone1"},
		AllocatableIPLow:  net.ParseIP("10.10.0.10"),
		AllocatableIPHigh: net.ParseIP("10.10.0.100"),
	}, {
		CIDR:              "2001:db8::/32",
		ProviderId:        "sn-ipv6",
		AvailabilityZones: []string{"zone1", "zone3"},
	}, {
		CIDR:       "",
		ProviderId: "",
		// no CIDR or provider id -> cached, but cannot be added
	}, {
		CIDR:       "",
		ProviderId: "sn-empty",
		// no CIDR, just provider id -> cached, but can only be added by id
	}, {
		CIDR:       "invalid",
		ProviderId: "sn-invalid",
		// invalid CIDR and provider id -> cannot be added, but is cached
	}, {
		CIDR:       "0.1.2.3/4",
		ProviderId: "sn-awesome",
		// incorrectly specified CIDR, with provider id -> cached, cannot be added
	}, {
		CIDR: "10.20.0.0/16",
		// no zones, no provider-id -> cached, but can only be added by CIDR
	}, {
		CIDR:              "10.99.88.0/24",
		ProviderId:        "sn-deadbeef",
		AvailabilityZones: []string{"zone1", "zone2"},
		// with zones, duplicate provider-id -> overwritten by the last
		// subnet with the same provider id when caching.
	}, {
		CIDR:       "10.42.0.0/16",
		ProviderId: "sn-42",
		// no zones
	}, {
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-deadbeef",
		AvailabilityZones: []string{"zone2"},
		// in an unavailable zone, duplicate CIDR -> cannot be added, but is cached
	}, {
		CIDR:              "10.30.1.0/24",
		ProviderId:        "vlan-42",
		VLANTag:           42,
		AvailabilityZones: []string{"zone3"},
	}}

	environs.RegisterProvider(StubProviderType, ProviderInstance)
}

// StubMethodCall is like testing.StubCall, but includes the receiver
// as well.
type StubMethodCall struct {
	Receiver interface{}
	FuncName string
	Args     []interface{}
}

// BackingCall makes it easy to check method calls on BackingInstance.
func BackingCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: BackingInstance,
		FuncName: name,
		Args:     args,
	}
}

// ProviderCall makes it easy to check method calls on ProviderInstance.
func ProviderCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: ProviderInstance,
		FuncName: name,
		Args:     args,
	}
}

// EnvironCall makes it easy to check method calls on EnvironInstance.
func EnvironCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: EnvironInstance,
		FuncName: name,
		Args:     args,
	}
}

// ZonedEnvironCall makes it easy to check method calls on
// ZonedEnvironInstance.
func ZonedEnvironCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: ZonedEnvironInstance,
		FuncName: name,
		Args:     args,
	}
}

// NetworkingEnvironCall makes it easy to check method calls on
// NetworkingEnvironInstance.
func NetworkingEnvironCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: NetworkingEnvironInstance,
		FuncName: name,
		Args:     args,
	}
}

// ZonedNetworkingEnvironCall makes it easy to check method calls on
// ZonedNetworkingEnvironInstance.
func ZonedNetworkingEnvironCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: ZonedNetworkingEnvironInstance,
		FuncName: name,
		Args:     args,
	}
}

// CheckMethodCalls works like testing.Stub.CheckCalls, but also
// checks the receivers.
func CheckMethodCalls(c *gc.C, stub *testing.Stub, calls ...StubMethodCall) {
	receivers := make([]interface{}, len(calls))
	for i, call := range calls {
		receivers[i] = call.Receiver
	}
	stub.CheckReceivers(c, receivers...)
	c.Check(stub.Calls(), gc.HasLen, len(calls))
	for i, call := range calls {
		stub.CheckCall(c, i, call.FuncName, call.Args...)
	}
}

// FakeZone implements providercommon.AvailabilityZone for testing.
type FakeZone struct {
	name      string
	available bool
}

var _ providercommon.AvailabilityZone = (*FakeZone)(nil)

func (f *FakeZone) Name() string {
	return f.name
}

func (f *FakeZone) Available() bool {
	return f.available
}

// GoString implements fmt.GoStringer.
func (f *FakeZone) GoString() string {
	return fmt.Sprintf("&FakeZone{%q, %v}", f.name, f.available)
}

// FakeSpace implements subnets.BackingSpace for testing.
type FakeSpace struct {
	name string
}

var _ subnets.BackingSpace = (*FakeSpace)(nil)

func (f *FakeSpace) Name() string {
	return f.name
}

// GoString implements fmt.GoStringer.
func (f *FakeSpace) GoString() string {
	return fmt.Sprintf("&FakeSpace{%q}", f.name)
}

// FakeSubnet implements subnets.BackingSubnet for testing.
type FakeSubnet struct {
	info subnets.BackingSubnetInfo
}

var _ subnets.BackingSubnet = (*FakeSubnet)(nil)

// GoString implements fmt.GoStringer.
func (f *FakeSubnet) GoString() string {
	return fmt.Sprintf("&FakeSubnet{%#v}", f.info)
}

// ResetStub resets all recorded calls and errors of the given stub.
func ResetStub(stub *testing.Stub) {
	*stub = testing.Stub{}
}

// StubBacking implements subnets.Backing and records calls to its
// methods.
type StubBacking struct {
	*testing.Stub

	EnvConfig *config.Config

	Zones   []providercommon.AvailabilityZone
	Spaces  []subnets.BackingSpace
	Subnets []subnets.BackingSubnet
}

var _ subnets.Backing = (*StubBacking)(nil)

type SetUpFlag bool

const (
	WithZones      SetUpFlag = true
	WithoutZones   SetUpFlag = false
	WithSpaces     SetUpFlag = true
	WithoutSpaces  SetUpFlag = false
	WithSubnets    SetUpFlag = true
	WithoutSubnets SetUpFlag = false
)

func (sb *StubBacking) SetUp(c *gc.C, envName string, withZones, withSpaces, withSubnets SetUpFlag) {
	// This method should be called at the beginning of each test, so
	// reset the recorded calls and errors.
	ResetStub(sb.Stub)

	// Make sure we use the stub provider.
	extraAttrs := coretesting.Attrs{
		"uuid": utils.MustNewUUID().String(),
		"type": StubProviderType,
		"name": envName,
	}
	sb.EnvConfig = coretesting.CustomEnvironConfig(c, extraAttrs)
	sb.Zones = []providercommon.AvailabilityZone{}
	if withZones {
		sb.Zones = make([]providercommon.AvailabilityZone, len(ProviderInstance.Zones))
		copy(sb.Zones, ProviderInstance.Zones)
	}
	sb.Spaces = []subnets.BackingSpace{}
	if withSpaces {
		sb.Spaces = []subnets.BackingSpace{
			&FakeSpace{"default"},
			&FakeSpace{"dmz"},
			&FakeSpace{"private"},
			&FakeSpace{"private"}, // duplicates are ignored when caching spaces.
		}
	}
	sb.Subnets = []subnets.BackingSubnet{}
	if withSubnets {
		info0 := subnets.BackingSubnetInfo{
			CIDR:              ProviderInstance.Subnets[0].CIDR,
			ProviderId:        string(ProviderInstance.Subnets[0].ProviderId),
			AllocatableIPLow:  ProviderInstance.Subnets[0].AllocatableIPLow.String(),
			AllocatableIPHigh: ProviderInstance.Subnets[0].AllocatableIPHigh.String(),
			AvailabilityZones: ProviderInstance.Subnets[0].AvailabilityZones,
			SpaceName:         "private",
		}
		info1 := subnets.BackingSubnetInfo{
			CIDR:              ProviderInstance.Subnets[1].CIDR,
			ProviderId:        string(ProviderInstance.Subnets[1].ProviderId),
			AvailabilityZones: ProviderInstance.Subnets[1].AvailabilityZones,
			SpaceName:         "dmz",
		}

		sb.Subnets = []subnets.BackingSubnet{
			&FakeSubnet{info0},
			&FakeSubnet{info1},
		}
	}
}

func (sb *StubBacking) EnvironConfig() (*config.Config, error) {
	sb.MethodCall(sb, "EnvironConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.EnvConfig, nil
}

func (sb *StubBacking) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	sb.MethodCall(sb, "AvailabilityZones")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.Zones, nil
}

func (sb *StubBacking) SetAvailabilityZones(zones []providercommon.AvailabilityZone) error {
	sb.MethodCall(sb, "SetAvailabilityZones", zones)
	return sb.NextErr()
}

func (sb *StubBacking) AllSpaces() ([]subnets.BackingSpace, error) {
	sb.MethodCall(sb, "AllSpaces")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.Spaces, nil
}

func (sb *StubBacking) AddSubnet(subnetInfo subnets.BackingSubnetInfo) (subnets.BackingSubnet, error) {
	sb.MethodCall(sb, "AddSubnet", subnetInfo)
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	fs := &FakeSubnet{info: subnetInfo}
	sb.Subnets = append(sb.Subnets, fs)
	return fs, nil
}

// GoString implements fmt.GoStringer.
func (se *StubBacking) GoString() string {
	return "&StubBacking{}"
}

// StubProvider implements a subset of environs.EnvironProvider
// methods used in tests.
type StubProvider struct {
	*testing.Stub

	Zones   []providercommon.AvailabilityZone
	Subnets []network.SubnetInfo

	environs.EnvironProvider // panic on any not implemented method call.
}

var _ environs.EnvironProvider = (*StubProvider)(nil)

func (sp *StubProvider) Open(cfg *config.Config) (environs.Environ, error) {
	sp.MethodCall(sp, "Open", cfg)
	if err := sp.NextErr(); err != nil {
		return nil, err
	}
	switch cfg.Name() {
	case StubEnvironName:
		return EnvironInstance, nil
	case StubZonedEnvironName:
		return ZonedEnvironInstance, nil
	case StubNetworkingEnvironName:
		return NetworkingEnvironInstance, nil
	case StubZonedNetworkingEnvironName:
		return ZonedNetworkingEnvironInstance, nil
	}
	panic("unexpected environment name: " + cfg.Name())
}

// GoString implements fmt.GoStringer.
func (se *StubProvider) GoString() string {
	return "&StubProvider{}"
}

// StubEnviron is used in tests where environs.Environ is needed.
type StubEnviron struct {
	*testing.Stub

	environs.Environ // panic on any not implemented method call
}

var _ environs.Environ = (*StubEnviron)(nil)

// GoString implements fmt.GoStringer.
func (se *StubEnviron) GoString() string {
	return "&StubEnviron{}"
}

// StubZonedEnviron is used in tests where providercommon.ZonedEnviron
// is needed.
type StubZonedEnviron struct {
	*testing.Stub

	providercommon.ZonedEnviron // panic on any not implemented method call
}

var _ providercommon.ZonedEnviron = (*StubZonedEnviron)(nil)

func (se *StubZonedEnviron) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	se.MethodCall(se, "AvailabilityZones")
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Zones, nil
}

// GoString implements fmt.GoStringer.
func (se *StubZonedEnviron) GoString() string {
	return "&StubZonedEnviron{}"
}

// StubNetworkingEnviron is used in tests where
// environs.NetworkingEnviron is needed.
type StubNetworkingEnviron struct {
	*testing.Stub

	environs.NetworkingEnviron // panic on any not implemented method call
}

var _ environs.NetworkingEnviron = (*StubNetworkingEnviron)(nil)

func (se *StubNetworkingEnviron) Subnets(instId instance.Id, subIds []network.Id) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", instId, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

// GoString implements fmt.GoStringer.
func (se *StubNetworkingEnviron) GoString() string {
	return "&StubNetworkingEnviron{}"
}

// StubZonedNetworkingEnviron is used in tests where features from
// both environs.Networking and providercommon.ZonedEnviron are
// needed.
type StubZonedNetworkingEnviron struct {
	*testing.Stub

	// panic on any not implemented method call
	providercommon.ZonedEnviron
	environs.Networking
}

// GoString implements fmt.GoStringer.
func (se *StubZonedNetworkingEnviron) GoString() string {
	return "&StubZonedNetworkingEnviron{}"
}

func (se *StubZonedNetworkingEnviron) Subnets(instId instance.Id, subIds []network.Id) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", instId, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

func (se *StubZonedNetworkingEnviron) AvailabilityZones() ([]providercommon.AvailabilityZone, error) {
	se.MethodCall(se, "AvailabilityZones")
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Zones, nil
}
