// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package model_test -destination service_mock_test.go github.com/juju/juju/apiserver/common/model MachineService,ModelConfigService,StatusService

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
