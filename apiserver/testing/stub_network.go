// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdcontext "context"
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
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

type errReturner func() error

// FakeSpace implements networkingcommon.BackingSpace for testing.
type FakeSpace struct {
	SpaceId   string
	SpaceName string
	SubnetIds []string
	Public    bool
	NextErr   errReturner
}

var _ networkingcommon.BackingSpace = (*FakeSpace)(nil)

func (f *FakeSpace) Id() string {
	return f.SpaceId
}

func (f *FakeSpace) Name() string {
	return f.SpaceName
}

func (f *FakeSpace) NetworkSpace() (network.SpaceInfo, error) {
	if err := f.NextErr(); err != nil {
		return network.SpaceInfo{}, err
	}

	outputSpaceInfo := network.SpaceInfo{
		ID:   f.SpaceId,
		Name: network.SpaceName(f.SpaceName),
	}

	outputSpaceInfo.Subnets = make(network.SubnetInfos, len(f.SubnetIds))
	for i, subnetId := range f.SubnetIds {
		providerId := network.Id("provider-" + subnetId)

		// Pick the third element of the IP address and use this to
		// decide how we construct the Subnet. It provides variation of
		// test data.
		first, err := strconv.Atoi(strings.Split(subnetId, ".")[2])
		if err != nil {
			return outputSpaceInfo, err
		}
		vlantag := 0
		zones := []string{"foo"}
		if first%2 == 1 {
			vlantag = 23
			zones = []string{"bar", "bam"}
		}

		backing := network.SubnetInfo{
			CIDR:              subnetId,
			SpaceID:           f.SpaceId,
			SpaceName:         f.SpaceName,
			ProviderId:        providerId,
			VLANTag:           vlantag,
			AvailabilityZones: zones,
		}
		outputSpaceInfo.Subnets[i] = backing
	}

	return outputSpaceInfo, nil
}

func (f *FakeSpace) ProviderId() (netID network.Id) {
	return
}

func (f *FakeSpace) Zones() []string {
	return []string{""}
}

func (f *FakeSpace) Life() (life life.Value) {
	return
}

func (f *FakeSpace) EnsureDead() error {
	return nil
}

func (f *FakeSpace) Remove() error {
	return nil
}

// GoString implements fmt.GoStringer.
func (f *FakeSpace) GoString() string {
	return fmt.Sprintf("&FakeSpace{%q}", f.SpaceName)
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

// FakeSubnet implements networkingcommon.BackingSubnet for testing.
type FakeSubnet struct {
	Info networkingcommon.BackingSubnetInfo
	id   string
}

var _ networkingcommon.BackingSubnet = (*FakeSubnet)(nil)

// GoString implements fmt.GoStringer.
func (f *FakeSubnet) GoString() string {
	return fmt.Sprintf("&FakeSubnet{%#v}", f.Info)
}

func (f *FakeSubnet) CIDR() string {
	return f.Info.CIDR
}

func (f *FakeSubnet) ID() string {
	return f.id
}

func (f *FakeSubnet) AvailabilityZones() []string {
	return f.Info.AvailabilityZones
}

func (f *FakeSubnet) ProviderId() network.Id {
	return f.Info.ProviderId
}

func (f *FakeSubnet) ProviderNetworkId() network.Id {
	return f.Info.ProviderNetworkId
}

func (f *FakeSubnet) VLANTag() int {
	return f.Info.VLANTag
}

func (f *FakeSubnet) SpaceName() string {
	return f.Info.SpaceName
}

func (f *FakeSubnet) SpaceID() string {
	return f.Info.SpaceID
}

func (f *FakeSubnet) Life() state.Life {
	return state.LifeFromValue(f.Info.Life)
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

	Zones   network.AvailabilityZones
	Spaces  []networkingcommon.BackingSpace
	Subnets []networkingcommon.BackingSubnet
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
		"uuid": utils.MustNewUUID().String(),
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
	sb.Spaces = []networkingcommon.BackingSpace{}
	if withSpaces {
		// Note that full subnet data is generated from the SubnetIds in
		// FakeSpace.Subnets().
		sb.Spaces = []networkingcommon.BackingSpace{
			&FakeSpace{
				SpaceId:   "1",
				SpaceName: "default",
				SubnetIds: []string{"192.168.0.0/24", "192.168.3.0/24"},
				NextErr:   sb.NextErr},
			&FakeSpace{
				SpaceId:   "2",
				SpaceName: "dmz",
				SubnetIds: []string{"192.168.1.0/24"},
				NextErr:   sb.NextErr},
			&FakeSpace{
				SpaceId:   "3",
				SpaceName: "private",
				SubnetIds: []string{"192.168.2.0/24"},
				NextErr:   sb.NextErr},
			&FakeSpace{
				SpaceId:   "4",
				SpaceName: "private",
				SubnetIds: []string{"192.168.2.0/24"},
				NextErr:   sb.NextErr}, // duplicates are ignored when caching spaces.
		}
	}
	sb.Subnets = []networkingcommon.BackingSubnet{}
	if withSubnets {
		info0 := networkingcommon.BackingSubnetInfo{
			CIDR:              ProviderInstance.Subnets[0].CIDR,
			ProviderId:        ProviderInstance.Subnets[0].ProviderId,
			ProviderNetworkId: ProviderInstance.Subnets[0].ProviderNetworkId,
			AvailabilityZones: ProviderInstance.Subnets[0].AvailabilityZones,
			SpaceName:         "private",
			SpaceID:           "3",
		}
		info1 := networkingcommon.BackingSubnetInfo{
			CIDR:              ProviderInstance.Subnets[1].CIDR,
			ProviderId:        ProviderInstance.Subnets[1].ProviderId,
			ProviderNetworkId: ProviderInstance.Subnets[1].ProviderNetworkId,
			AvailabilityZones: ProviderInstance.Subnets[1].AvailabilityZones,
			SpaceName:         "dmz",
			SpaceID:           "2",
		}
		sb.Subnets = []networkingcommon.BackingSubnet{
			&FakeSubnet{Info: info0, id: info0.SpaceID},
			&FakeSubnet{Info: info1, id: info1.SpaceID},
		}
	}
}

func (sb *StubBacking) ControllerUUID() string {
	return "dummy-controller-uuid"
}

func (sb *StubBacking) AdditionalSubnets() {
	for i, info := range ProviderInstance.Subnets[10:] {
		sb.Subnets = append(sb.Subnets, &FakeSubnet{
			Info: networkingcommon.BackingSubnetInfo{
				CIDR:              info.CIDR,
				ProviderId:        info.ProviderId,
				ProviderNetworkId: info.ProviderNetworkId,
				AvailabilityZones: info.AvailabilityZones},
			id: strconv.Itoa(i + 30),
		},
		)
	}
}

func (sb *StubBacking) ModelConfig() (*config.Config, error) {
	sb.MethodCall(sb, "ModelConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return sb.EnvConfig, nil
}

func (sb *StubBacking) ModelTag() names.ModelTag {
	return names.NewModelTag("dbeef-2f18-4fd2-967d-db9663db7bea")
}

func (sb *StubBacking) CloudSpec() (environscloudspec.CloudSpec, error) {
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

func (sb *StubBacking) AllSpaces() ([]networkingcommon.BackingSpace, error) {
	sb.MethodCall(sb, "AllSpaces")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}

	// Filter duplicates.
	seen := set.Strings{}
	output := []networkingcommon.BackingSpace{}
	for _, space := range sb.Spaces {
		if seen.Contains(space.Name()) {
			continue
		}
		seen.Add(space.Name())
		output = append(output, space)
	}
	return output, nil
}

func (sb *StubBacking) AllSubnets() ([]networkingcommon.BackingSubnet, error) {
	sb.MethodCall(sb, "AllSubnets")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}

	// Filter duplicates.
	seen := set.Strings{}
	output := []networkingcommon.BackingSubnet{}
	for _, subnet := range sb.Subnets {
		if seen.Contains(subnet.CIDR()) {
			continue
		}
		seen.Add(subnet.CIDR())
		output = append(output, subnet)
	}
	return output, nil
}

func (sb *StubBacking) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	sb.MethodCall(sb, "SubnetByCIDR", cidr)
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	for _, subnet := range sb.Subnets {
		if subnet.CIDR() == cidr {
			return subnet, nil
		}
	}
	return nil, errors.NewNotFound(nil, fmt.Sprintf("subnet %q", cidr))
}

func (sb *StubBacking) AddSubnet(subnetInfo networkingcommon.BackingSubnetInfo) (networkingcommon.BackingSubnet, error) {
	sb.MethodCall(sb, "AddSubnet", subnetInfo)
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	fs := &FakeSubnet{Info: subnetInfo}
	sb.Subnets = append(sb.Subnets, fs)
	return fs, nil
}

func (sb *StubBacking) AddSpace(name string, providerId network.Id, subnets []string, public bool) (networkingcommon.BackingSpace, error) {
	sb.MethodCall(sb, "AddSpace", name, providerId, subnets, public)
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	fs := &FakeSpace{SpaceName: name, SubnetIds: subnets, Public: public}
	sb.Spaces = append(sb.Spaces, fs)
	return fs, nil
}

func (sb *StubBacking) SaveProviderSubnets(subnets []network.SubnetInfo, spaceID string) error {
	sb.MethodCall(sb, "SaveProviderSubnets", subnets, spaceID)
	if err := sb.NextErr(); err != nil {
		return err
	}
	return nil
}

func (sb *StubBacking) AllEndpointBindingsSpaceNames() (set.Strings, error) {
	sb.MethodCall(sb, "AllEndpointBindingsSpaceNames")
	return set.NewStrings(), nil
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

func (sp *StubProvider) Open(_ stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	sp.MethodCall(sp, "Open", args.Config)
	if err := sp.NextErr(); err != nil {
		return nil, err
	}
	switch args.Config.Name() {
	case StubEnvironName:
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

func (se *StubZonedEnviron) AvailabilityZones(ctx context.ProviderCallContext) (network.AvailabilityZones, error) {
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
	ctx context.ProviderCallContext, instId instance.Id, subIds []network.Id,
) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", ctx, instId, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

func (se *StubNetworkingEnviron) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	se.MethodCall(se, "SupportsSpaces", ctx)
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

func (se *StubZonedNetworkingEnviron) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	se.MethodCall(se, "SupportsSpaces", ctx)
	if err := se.NextErr(); err != nil {
		return false, err
	}
	return true, nil
}

func (se *StubZonedNetworkingEnviron) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
	se.MethodCall(se, "SupportsSpaceDiscovery", ctx)
	if err := se.NextErr(); err != nil {
		return false, err
	}
	return true, nil
}

func (se *StubZonedNetworkingEnviron) Subnets(
	ctx context.ProviderCallContext, instId instance.Id, subIds []network.Id,
) ([]network.SubnetInfo, error) {
	se.MethodCall(se, "Subnets", ctx, instId, subIds)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Subnets, nil
}

func (se *StubZonedNetworkingEnviron) AvailabilityZones(ctx context.ProviderCallContext) (network.AvailabilityZones, error) {
	se.MethodCall(se, "AvailabilityZones", ctx)
	if err := se.NextErr(); err != nil {
		return nil, err
	}
	return ProviderInstance.Zones, nil
}
