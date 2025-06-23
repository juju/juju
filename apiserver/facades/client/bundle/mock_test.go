// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/description/v9"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type mockCharm struct {
	charm.Charm
}

func (c *mockCharm) Config() *charm.Config {
	return &charm.Config{Options: map[string]charm.Option{
		"foo": {Default: "bar"},
	}}
}

type mockState struct {
	testing.Stub
	bundle.Backend
	model  description.Model
	charm  *mockCharm
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

func (m *mockState) Charm(url string) (charm.Charm, error) {
	return m.charm, nil
}

func (m *mockState) AllSpaceInfos() (network.SpaceInfos, error) {
	result := make(network.SpaceInfos, len(m.Spaces))
	i := 0
	for id, name := range m.Spaces {
		result[i] = network.SpaceInfo{ID: id, Name: network.SpaceName(name)}
		i += 1
	}
	return result, nil
}

func (m *mockState) Space(_ string) (*state.Space, error) {
	return nil, nil
}

func newMockState() *mockState {
	st := &mockState{
		Stub: testing.Stub{},
	}
	st.Spaces = make(map[string]string)
	return st
}
