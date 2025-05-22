// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package computeprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package computeprovisioner_test -destination provisioner_mock_test.go github.com/juju/juju/internal/worker/computeprovisioner ControllerAPI,MachinesAPI,MachineService
//go:generate go run go.uber.org/mock/mockgen -typed -package computeprovisioner_test -destination dependency_mock_test.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -typed -package computeprovisioner_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package computeprovisioner_test -destination base_mock_test.go github.com/juju/juju/api/base APICaller
