// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/description"
	"github.com/juju/testing"
	"gopkg.in/juju/names.v2"
)

type mockState struct {
	testing.Stub
}

func (m *mockState) Export() (description.Model, error) {
	m.MethodCall(m, "Export")
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	args := description.ModelArgs{
		Owner: names.NewUserTag("read"),
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
