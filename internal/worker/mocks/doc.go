// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This package contains common worker mocks.
// run 'go generate' to regenerate the mock interfaces

package mocks

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination worker_mock.go github.com/juju/worker/v4 Worker
