// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/api/agent/reboot Client
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/lock_mock.go github.com/juju/juju/core/machinelock Lock
