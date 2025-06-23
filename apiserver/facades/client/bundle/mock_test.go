// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/description/v10"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
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
	testhelpers.Stub
	bundle.Backend
	model  description.Model
	charm  *mockCharm
	Spaces map[string]string
}

func (m *mockState) ExportPartial(config state.ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
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
		SkipLinkLayerDevices:   true,
	}
}

func (m *mockState) Charm(url string) (charm.Charm, error) {
	return m.charm, nil
}

func newMockState() *mockState {
	st := &mockState{
		Stub: testhelpers.Stub{},
	}
	st.Spaces = make(map[string]string)
	return st
}

type mockObjectStore struct {
	objectstore.ObjectStore
}
