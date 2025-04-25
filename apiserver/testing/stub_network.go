// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	providercommon "github.com/juju/juju/internal/provider/common"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type StubNetwork struct {
}

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

const (
	StubProviderType               = "stub-provider"
	StubEnvironName                = "stub-environ"
	StubZonedEnvironName           = "stub-zoned-environ"
	StubNetworkingEnvironName      = "stub-networking-environ"
	StubZonedNetworkingEnvironName = "stub-zoned-networking-environ"
)

func (s StubNetwork) SetUpSuite(c *gc.C) {
	providers := environs.RegisteredProviders()
	for _, name := range providers {
		if name == StubProviderType {
			return
		}
	}

	ProviderInstance.Zones = network.AvailabilityZones{
		&FakeZone{"zone1", true},
		&FakeZone{"zone2", false},
		&FakeZone{"zone3", true},
		&FakeZone{"zone4", false},
		&FakeZone{"zone4", false}, // duplicates are ignored
	}
	ProviderInstance.Subnets = []network.SubnetInfo{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-zadf00d",
		ProviderNetworkId: "godspeed",
		AvailabilityZones: []string{"zone1"},
	}, {
		CIDR:              "2001:db8::/32",
		ProviderId:        "sn-ipv6",
		AvailabilityZones: []string{"zone1", "zone3"},
	}, {
		// no CIDR or provider id -> cached, but cannot be added
		CIDR:       "",
		ProviderId: "",
	}, {
		// no CIDR, just provider id -> cached, but can only be added by id
		CIDR:       "",
		ProviderId: "sn-empty",
	}, {
		// invalid CIDR and provider id -> cannot be added, but is cached
		CIDR:       "invalid",
		ProviderId: "sn-invalid",
	}, {
		// incorrectly specified CIDR, with provider id -> cached, cannot be added
		CIDR:       "0.1.2.3/4",
		ProviderId: "sn-awesome",
	}, {
		// no zones, no provider-id -> cached, but can only be added by CIDR
		CIDR: "10.20.0.0/16",
	}, {
		// with zones, duplicate provider-id -> overwritten by the last
		// subnet with the same provider id when caching.
		CIDR:              "10.99.88.0/24",
		ProviderId:        "sn-deadbeef",
		AvailabilityZones: []string{"zone1", "zone2"},
	}, {
		// no zones
		CIDR:       "10.42.0.0/16",
		ProviderId: "sn-42",
	}, {
		// in an unavailable zone, duplicate CIDR -> cannot be added, but is cached
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-deadbeef",
		AvailabilityZones: []string{"zone2"},
	}, {
		CIDR:              "10.30.1.0/24",
		ProviderId:        "vlan-42",
		VLANTag:           42,
		AvailabilityZones: []string{"zone3"},
	}, {
		CIDR:              "10.0.2.0/24",
		ProviderId:        "sn-zadf00d-2",
		ProviderNetworkId: "godspeed-2",
		AvailabilityZones: []string{"zone1"},
	}, {
		CIDR:              "10.0.3.0/24",
		ProviderId:        "sn-zadf00d-3",
		ProviderNetworkId: "godspeed-3",
		AvailabilityZones: []string{"zone1"},
	}, {
		CIDR:              "10.0.4.0/24",
		ProviderId:        "sn-zadf00d-4",
		ProviderNetworkId: "godspeed-4",
		AvailabilityZones: []string{"zone1"},
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

// ZonedEnvironCall makes it easy to check method calls on
// ZonedEnvironInstance.
func ZonedEnvironCall(name string, args ...interface{}) StubMethodCall {
	return StubMethodCall{
		Receiver: ZonedEnvironInstance,
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
	ZoneName      string
	ZoneAvailable bool
}

var _ network.AvailabilityZone = (*FakeZone)(nil)

func (f *FakeZone) Name() string {
	return f.ZoneName
}

func (f *FakeZone) Available() bool {
	return f.ZoneAvailable
}

// GoString implements fmt.GoStringer.
func (f *FakeZone) GoString() string {
	return fmt.Sprintf("&FakeZone{%q, %v}", f.ZoneName, f.ZoneAvailable)
}

// ResetStub resets all recorded calls and errors of the given stub.
func ResetStub(stub *testing.Stub) {
	*stub = testing.Stub{}
}

// StubBacking implements networkingcommon.NetworkBacking and records calls to its
// methods.
type StubBacking struct {
	*testing.Stub

	EnvConfig *config.Config
	Cloud     environscloudspec.CloudSpec

	Zones network.AvailabilityZones
}

var _ networkingcommon.NetworkBacking = (*StubBacking)(nil)

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
	// This method must be called at the beginning of each test, which
	// needs access to any of the mocks, to reset the recorded calls
	// and errors, as well as to initialize the mocks as needed.
	ResetStub(sb.Stub)

	// Make sure we use the stub provider.
	extraAttrs := coretesting.Attrs{
		"uuid": uuid.MustNewUUID().String(),
		"type": StubProviderType,
		"name": envName,
	}
	sb.EnvConfig = coretesting.CustomModelConfig(c, extraAttrs)
	sb.Cloud = environscloudspec.CloudSpec{
		Type:             StubProviderType,
		Name:             "cloud-name",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
	}
	sb.Zones = network.AvailabilityZones{}
	if withZones {
		sb.Zones = make(network.AvailabilityZones, len(ProviderInstance.Zones))
		copy(sb.Zones, ProviderInstance.Zones)
	}
}

func (sb *StubBacking) ModelConfig(_ context.Context) (*config.Config, error) {
	sb.MethodCall(sb, "ModelConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.EnvConfig, nil
}

func (sb *StubBacking) ModelTag() names.ModelTag {
	return names.NewModelTag("dbeef-2f18-4fd2-967d-db9663db7bea")
}

func (sb *StubBacking) CloudSpec(_ context.Context) (environscloudspec.CloudSpec, error) {
	sb.MethodCall(sb, "CloudSpec")
	if err := sb.NextErr(); err != nil {
		return environscloudspec.CloudSpec{}, err
	}
	return sb.Cloud, nil
}

func (sb *StubBacking) AvailabilityZones() (network.AvailabilityZones, error) {
	sb.MethodCall(sb, "AvailabilityZones")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.Zones, nil
}

func (sb *StubBacking) SetAvailabilityZones(zones network.AvailabilityZones) error {
	sb.MethodCall(sb, "SetAvailabilityZones", zones)
	return sb.NextErr()
}

func (sb *StubBacking) SaveProviderSubnets(subnets []network.SubnetInfo, spaceID string) error {
	sb.MethodCall(sb, "SaveProviderSubnets", subnets, spaceID)
	if err := sb.NextErr(); err != nil {
		return err
	}
	return nil
}

func (sb *StubBacking) DefaultEndpointBindingSpace() (string, error) {
	sb.MethodCall(sb, "DefaultEndpointBindingSpace")
	return "alpha", nil
}

// GoString implements fmt.GoStringer.
func (sb *StubBacking) GoString() string {
	return "&StubBacking{}"
}

// StubProvider implements a subset of environs.EnvironProvider
// methods used in tests.
type StubProvider struct {
	*testing.Stub

	Zones   network.AvailabilityZones
	Subnets []network.SubnetInfo

	environs.EnvironProvider // panic on any not implemented method call.
}

var _ environs.EnvironProvider = (*StubProvider)(nil)

func (sp *StubProvider) Open(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (environs.Environ, error) {
	sp.MethodCall(sp, "Open", args.Config)
	if err := sp.NextErr(); err != nil {
		return nil, err
	}
	name := args.Config.Name()
	if strings.HasPrefix(name, "testmodel-") {
		return EnvironInstance, nil
	}
	switch name {
	case StubEnvironName, bootstrap.ControllerModelName:
		return EnvironInstance, nil
	case StubZonedEnvironName:
		return ZonedEnvironInstance, nil
	case StubNetworkingEnvironName:
		return NetworkingEnvironInstance, nil
	case StubZonedNetworkingEnvironName:
		return ZonedNetworkingEnvironInstance, nil
	}
	panic("unexpected model name: " + args.Config.Name())
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

func (se *StubEnviron) Config() *config.Config {
	attrs := coretesting.FakeConfig()
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		panic(err)
	}
	return cfg
}

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

func (se *StubZonedEnviron) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	se.MethodCall(se, "AvailabilityZones", ctx)
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

func (se *StubNetworkingEnviron) Subnets(
	ctx envcontext.ProviderCallContext, subIds []network.Id,
) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", ctx, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

func (se *StubNetworkingEnviron) SupportsSpaces() (bool, error) {
	se.MethodCall(se, "SupportsSpaces")
	if err := se.NextErr(); err != nil {
		return false, err
	}
	return true, nil
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

func (se *StubZonedNetworkingEnviron) SupportsSpaces(ctx envcontext.ProviderCallContext) (bool, error) {
	se.MethodCall(se, "SupportsSpaces", ctx)
	if err := se.NextErr(); err != nil {
		return false, err
	}
	return true, nil
}

func (se *StubZonedNetworkingEnviron) SupportsSpaceDiscovery(ctx envcontext.ProviderCallContext) (bool, error) {
	se.MethodCall(se, "SupportsSpaceDiscovery", ctx)
	if err := se.NextErr(); err != nil {
		return false, err
	}
	return true, nil
}

func (se *StubZonedNetworkingEnviron) Subnets(
	ctx envcontext.ProviderCallContext, instId instance.Id, subIds []network.Id,
) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", ctx, instId, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

func (se *StubZonedNetworkingEnviron) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	se.MethodCall(se, "AvailabilityZones", ctx)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Zones, nil
}
