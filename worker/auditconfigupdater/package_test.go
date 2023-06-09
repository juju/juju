// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/changestream_mock.go github.com/juju/juju/core/changestream WatchableDBGetter

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
