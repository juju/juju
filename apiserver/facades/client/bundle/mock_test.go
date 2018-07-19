// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/description"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/state"
)

type mockState struct {
	testing.Stub
	bundle.Backend
}

func (m *mockState) Exportpartial(config state.ExportConfig) (description.Model, error) {
	m.SetExportconfig(config)

	m.MethodCall(m, "ExportPartial", config)
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	args := description.ModelArgs{
		Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region",
	}
	initial := description.NewModel(args)
	initial.SetStatus(description.StatusArgs{Value: "available"})

	return initial, nil
}

func newMockState() *mockState {
	st := &mockState{}
	return st
}
