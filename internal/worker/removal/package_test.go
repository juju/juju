// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

//go:generate go run github.com/canonical/gomock/mockgen -package removal -destination package_mocks_test.go github.com/juju/juju/internal/worker/removal RemovalService,Clock
//go:generate go run github.com/canonical/gomock/mockgen -package removal -destination timer_mocks_test.go github.com/juju/clock Timer
