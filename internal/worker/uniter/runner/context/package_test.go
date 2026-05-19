// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/internal/worker/uniter/runner/context LeadershipContext
