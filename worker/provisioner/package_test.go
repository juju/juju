// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/watcher.go github.com/juju/juju/core/watcher StringsWatcher

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
