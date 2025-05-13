// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// agentAuth implements facade.Authorizer for use in the tests.
type agentAuth struct {
	facade.Authorizer
	machine     bool
	unit        bool
	application bool
}

// AuthMachineAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthMachineAgent() bool {
	return auth.machine
}

// AuthUnitAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthUnitAgent() bool {
	return auth.unit
}

// AuthApplicationAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthApplicationAgent() bool {
	return auth.application
}

// newMockBackend returns a mock Backend that will add calls to the
// supplied testing.Stub, and return errors in the sequence it
// specifies.
func newMockBackend(stub *testhelpers.Stub) *mockBackend {
	return &mockBackend{
		stub: stub,
	}
}

// mockBackend implements migrationflag.Backend for use in the tests.
type mockBackend struct {
	stub *testhelpers.Stub
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
func (mock *mockBackend) WatchMigrationPhase() state.NotifyWatcher {
	mock.stub.AddCall("WatchMigrationPhase")
	return newMockWatcher(mock.stub)
}

// newMockWatcher consumes an error from the supplied testing.Stub, and
// returns a state.NotifyWatcher that either works or doesn't depending
// on whether the error was nil.
func newMockWatcher(stub *testhelpers.Stub) *mockWatcher {
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
