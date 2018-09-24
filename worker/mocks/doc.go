// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package contains common worker mocks.
// run 'go generate' to regenerate the mock interfaces

package mocks

//go:generate mockgen -package mocks -destination worker_mock.go gopkg.in/juju/worker.v1 Worker
