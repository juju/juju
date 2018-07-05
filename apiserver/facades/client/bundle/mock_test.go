// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/testing"
)

type mockState struct {
	testing.Stub
	bundle.Backend
	str params.StringResult
}

func (m *mockState) ExportBundle() (params.StringResult, error) {
	m.MethodCall(m, "ExportBundle")
	if err := m.NextErr(); err != nil {
		return params.StringResult{}, err
	}

	str := m.str
	return str, nil
}

func newMockState() *mockState {
	st := &mockState{}
	return st
}
