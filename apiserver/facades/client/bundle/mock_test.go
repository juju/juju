// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/testing"

	"github.com/juju/description"
)

type mockState struct {
	testing.Stub
	model mockModel
}

func (m *mockState) Export() (description.Model, error) {
	m.MethodCall(m, "Export")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.model.desc, nil
}

func newMockState() *mockState {
	st := &mockState{
		model: mockModel{},
	}
	return st
}

type mockModel struct {
	testing.Stub
	desc description.Model
}
