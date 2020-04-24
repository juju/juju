// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/names/v4"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

// mockAuth represents a machine which may or may not be a controller.
type mockAuth struct {
	facade.Authorizer
	nonController bool
}

// AuthModelManager is part of the facade.Authorizer interface.
func (mock mockAuth) AuthController() bool {
	return !mock.nonController
}

// GetAuthTag is part of the facade.Authorizer interface.
func (mockAuth) GetAuthTag() names.Tag {
	return names.NewMachineTag("123")
}

// mockBackend implements singular.Backend and lease.Claimer.
type mockBackend struct {
	stub testing.Stub
}

// ControllerTag is part of the singular.Backend interface.
func (mock *mockBackend) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

// ModelTag is part of the singular.Backend interface.
func (mock *mockBackend) ModelTag() names.ModelTag {
	return coretesting.ModelTag
}

// Claim is part of the lease.Claimer interface.
func (mock *mockBackend) Claim(lease, holder string, duration time.Duration) error {
	mock.stub.AddCall("Claim", lease, holder, duration)
	return mock.stub.NextErr()
}

// WaitUntilExpired is part of the lease.Claimer interface.
func (mock *mockBackend) WaitUntilExpired(leaseId string, cancel <-chan struct{}) error {
	mock.stub.AddCall("WaitUntilExpired", leaseId)
	select {
	case <-cancel:
		return lease.ErrWaitCancelled
	default:
	}
	return mock.stub.NextErr()
}
