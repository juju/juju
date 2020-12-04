// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package api exists because we can't generate mocks in the main namespace.
package api

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/cmd/plugins/juju-wait-for/api WatchAllAPI,AllWatcher

func Test(t *testing.T) {
	gc.TestingT(t)
}
