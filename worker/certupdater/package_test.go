// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/changestream_mock.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/worker/certupdater ControllerConfigService

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
