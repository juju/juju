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
	model  description.Model
	Spaces map[string]string
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

func (m *mockState) SpaceNamesByID() (map[string]string, error) {
	return m.Spaces, nil
}

func newMockState() *mockState {
	st := &mockState{
		Stub: testing.Stub{},
	}
	st.Spaces = make(map[string]string)
	return st
}
