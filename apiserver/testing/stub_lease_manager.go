// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/v2/core/lease"

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
