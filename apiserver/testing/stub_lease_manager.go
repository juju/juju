// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/core/lease"

// StubLeaseReader pretends to implement lease.Reader.
// The non-implemented Leases() method should not be called by tests in the
// base apiserver package.
type StubLeaseReader struct {
	lease.Reader
}

// StubLeaseManager pretends to implement lease.Manager.
// The only method called in this package should be Reader(),
// implemented below.
type StubLeaseManager struct {
	lease.Manager
}

// Reader returns a StubLeaseReader, which will panic if used.
func (m StubLeaseManager) Reader(namespace string, modelUUID string) (lease.Reader, error) {
	return StubLeaseReader{}, nil
}

// StubLeasePinner pretends to implement lease.Pinner.
// The non-implemented methods should not be called by tests in the
// base apiserver package.
type StubLeasePinner struct {
	lease.Pinner
}

// Pinner returns a StubLeasePinner, which will panic if used.
func (m StubLeaseManager) Pinner(namespace string, modelUUID string) (lease.Pinner, error) {
	return StubLeasePinner{}, nil
}
