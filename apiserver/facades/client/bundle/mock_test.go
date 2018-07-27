// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/description"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	bundle.Backend
	model description.Model
	mac   *state.Machine
}

func (m *mockState) ExportPartial(config state.ExportConfig) (description.Model, error) {
	m.MethodCall(m, "ExportPartial", config)
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	return m.model, nil
}

func (m *mockState) GetExportConfig() state.ExportConfig {
	return state.ExportConfig{
		SkipActions:            true,
		SkipCloudImageMetadata: true,
		SkipCredentials:        true,
		SkipIPAddresses:        true,
		SkipSSHHostKeys:        true,
		SkipStatusHistory:      true,
		SkipLinkLayerDevices:   true,
	}
}

func (m *mockState) Machine(id string) (*state.Machine, error) {
	m.MethodCall(m, "Machine", id)
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	return m.mac, nil
}

func newMockState() *mockState {
	st := &mockState{
		Stub: testing.Stub{},
	}
	return st
}
