// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/status"
	"github.com/juju/testing"
)

type mockState struct {
	testing.Stub
	app mockApplication
}

func newMockState() *mockState {
	return &mockState{}
}

func (st *mockState) Application(id string) (caasoperator.Application, error) {
	st.MethodCall(st, "Application", id)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.app, nil
}

type mockApplication struct {
	testing.Stub
}

func (app *mockApplication) SetStatus(info status.StatusInfo) error {
	app.MethodCall(app, "SetStatus", info)
	return app.NextErr()
}
