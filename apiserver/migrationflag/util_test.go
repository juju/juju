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

// agentAuth implements common.Authorizer for use in the tests.
type agentAuth struct {
	common.Authorizer
	machine bool
	unit    bool
}

// AuthMachineAgent is part of the common.Authorizer interface.
func (auth agentAuth) AuthMachineAgent() bool {
	return auth.machine
}

// AuthUnitAgent is part of the common.Authorizer interface.
func (auth agentAuth) AuthUnitAgent() bool {
	return auth.unit
}

// newMockBackend returns a mock Backend that will add calls to the
// supplied testing.Stub, and return errors in the sequence it
// specifies.
func newMockBackend(stub *testing.Stub) *mockBackend {
	return &mockBackend{
		stub: stub,
	}
}

// mockBackend implements migrationflag.Backend for use in the tests.
type mockBackend struct {
	stub *testing.Stub
}

// ModelUUID is part of the migrationflag.Backend interface.
func (mock *mockBackend) ModelUUID() string {
	return coretesting.ModelTag.Id()
}

// MigrationPhase is part of the migrationflag.Backend interface.
func (mock *mockBackend) MigrationPhase() (migration.Phase, error) {
	mock.stub.AddCall("MigrationPhase")
	if err := mock.stub.NextErr(); err != nil {
		return migration.UNKNOWN, err
	}
	return migration.REAP, nil
}

// WatchMigrationPhase is part of the migrationflag.Backend interface.
func (mock *mockBackend) WatchMigrationPhase() (state.NotifyWatcher, error) {
	mock.stub.AddCall("WatchMigrationPhase")
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return newMockWatcher(mock.stub), nil
}

// newMockWatcher consumes an error from the supplied testing.Stub, and
// returns a state.NotifyWatcher that either works or doesn't depending
// on whether the error was nil.
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

// mockWatcher implements state.NotifyWatcher for use in the tests.
type mockWatcher struct {
	state.NotifyWatcher
	changes chan struct{}
	err     error
}

// Changes is part of the state.NotifyWatcher interface.
func (mock *mockWatcher) Changes() <-chan struct{} {
	return mock.changes
}

// Err is part of the state.NotifyWatcher interface.
func (mock *mockWatcher) Err() error {
	return mock.err
}

// entities is a convenience constructor for params.Entities.
func entities(tags ...string) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}

// authOK will always authenticate successfully.
var authOK = agentAuth{machine: true}

// unknownModel is expected to induce a permissions error.
const unknownModel = "model-01234567-89ab-cdef-0123-456789abcdef"
