// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type agentAuth struct {
	common.Authorizer
	machine bool
	unit    bool
}

var authOK = agentAuth{machine: true}

func (auth agentAuth) AuthMachineAgent() bool {
	return auth.machine
}

func (auth agentAuth) AuthUnitAgent() bool {
	return auth.unit
}

func newMockBackend(stub *testing.Stub) *mockBackend {
	return &mockBackend{
		stub: stub,
	}
}

type mockBackend struct {
	stub *testing.Stub
}

func (mock *mockBackend) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

func (mock *mockBackend) MigrationPhase() (migration.Phase, error) {
	mock.stub.AddCall("MigrationPhase")
	if err := mock.stub.NextErr(); err != nil {
		return migration.UNKNOWN, err
	}
	return migration.REAP, nil
}

func (mock *mockBackend) WatchMigrationPhase() (state.NotifyWatcher, error) {
	mock.stub.AddCall("WatchMigrationPhase")
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return newMockWatcher(mock.stub), nil
}

func newMockWatcher(stub *testing.Stub) *mockWatcher {
	changes := make(chan struct{}, 1)
	err := stub.NextErr()
	if err == nil {
		changes <- struct{}{}
	} else {
		close(changes)
	}
	return &mockWatcher{
		err:     err,
		changes: changes,
	}
}

type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
	err     error
}

func (mock *mockWatcher) Changes() <-chan struct{} {
	return mock.changes
}

func (mock *mockWatcher) Err() error {
	return mock.err
}

const unknownModel = "model-01234567-89ab-cdef-0123-456789abcdef"

func entities(tags ...string) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}
