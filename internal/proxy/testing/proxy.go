// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "context"

type MockProxier struct {
	// See Proxier interface
	RawConfigFn func() (map[string]interface{}, error)

	// See Proxier interface
	StartFn func() error

	// See Proxier interface
	StopFn func()

	// See Proxier interface
	TypeFn func() string
}

type MockTunnelProxier struct {
	*MockProxier

	// See TunnelProxier interface
	HostFn func() string

	// See TunnelProxier interface
	PortFn func() string
}

func NewMockTunnelProxier() *MockTunnelProxier {
	return &MockTunnelProxier{
		MockProxier: &MockProxier{},
	}
}

func (mp *MockProxier) RawConfig() (map[string]interface{}, error) {
	if mp.RawConfigFn == nil {
		return map[string]interface{}{}, nil
	}
	return mp.RawConfigFn()
}

func (mp *MockProxier) Start(_ context.Context) error {
	if mp.StartFn == nil {
		return nil
	}
	return mp.StartFn()
}

func (mp *MockProxier) MarshalYAML() (interface{}, error) { return nil, nil }

func (mp *MockProxier) Insecure() {}

func (mp *MockProxier) Stop() {
	if mp.StopFn != nil {
		mp.StopFn()
	}
}

func (mp *MockProxier) Type() string {
	if mp.TypeFn == nil {
		return "mock-proxier"
	}
	return mp.TypeFn()
}

func (mtp *MockTunnelProxier) Host() string {
	if mtp.HostFn == nil {
		return ""
	}
	return mtp.HostFn()
}

func (mtp *MockTunnelProxier) Port() string {
	if mtp.PortFn == nil {
		return ""
	}
	return mtp.PortFn()
}
