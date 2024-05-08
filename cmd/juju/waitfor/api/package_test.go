// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package api exists because we can't generate mocks in the main namespace.
package api

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/cmd/juju/waitfor/api WatchAllAPI,AllWatcher

func Test(t *testing.T) {
	gc.TestingT(t)
}
