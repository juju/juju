// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/testing"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/subnets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	providercommon "github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	StubProviderType     = "stub-provider"
	StubEnvironName      = "stub-environ"
	StubZonedEnvironName = "stub-zoned-environ"
)

var (
	// SharedStub records all method calls to any of the stubs.
	SharedStub = &testing.Stub{}

	BackingInstance      = &StubBacking{Stub: SharedStub}
	ProviderInstance     = &StubProvider{Stub: SharedStub}
	EnvironInstance      = &StubEnviron{Stub: SharedStub}
	ZonedEnvironInstance = &StubZonedEnviron{Stub: SharedStub}
)

func init() {
	ProviderInstance.Zones = []providercommon.AvailabilityZone{
		&FakeZone{"env-zone1", true},
		&FakeZone{"env-zone2", false},
	}
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

// CheckMethodCalls works like testing.Stub.CheckCalls, but also
// checks the receivers.
func CheckMethodCalls(c *gc.C, stub *testing.Stub, calls ...StubMethodCall) {
	receivers := make([]interface{}, len(calls))
	for i, call := range calls {
		receivers[i] = call.Receiver
	}
	if !stub.CheckReceivers(c, receivers...) {
		return
	}
	if !c.Check(stub.Calls(), gc.HasLen, len(calls)) {
		return
	}
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

// ResetStub resets all recorded calls and errors of the given stub.
func ResetStub(stub *testing.Stub) {
	*stub = testing.Stub{}
}

// StubBacking implements subnets.Backing and records calls to its
// methods.
type StubBacking struct {
	*testing.Stub

	EnvConfig *config.Config

	Zones  []providercommon.AvailabilityZone
	Spaces []subnets.BackingSpace
}

var _ subnets.Backing = (*StubBacking)(nil)

type SetUpFlag bool

const (
	WithZones     SetUpFlag = true
	WithoutZones  SetUpFlag = false
	WithSpaces    SetUpFlag = true
	WithoutSpaces SetUpFlag = true
)

func (sb *StubBacking) SetUp(c *gc.C, envName string, withZones, withSpaces SetUpFlag) {
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
		sb.Zones = []providercommon.AvailabilityZone{
			&FakeZone{"zone1", true},
			&FakeZone{"zone2", false},
			&FakeZone{"zone3", true},
		}
	}
	sb.Spaces = []subnets.BackingSpace{}
	if withSpaces {
		sb.Spaces = []subnets.BackingSpace{
			&FakeSpace{"default"},
			&FakeSpace{"dmz"},
			&FakeSpace{"private"},
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

// StubProvider implements a subset of environs.EnvironProvider
// methods used in tests.
type StubProvider struct {
	*testing.Stub

	Zones []providercommon.AvailabilityZone

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
	}
	panic("unexpected environment name: " + cfg.Name())
}

// StubEnviron is used in tests where environs.Environ is needed.
type StubEnviron struct {
	*testing.Stub

	environs.Environ // panic on any not implemented method call
}

var _ environs.Environ = (*StubEnviron)(nil)

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
