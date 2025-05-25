// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

//go:generate go run go.uber.org/mock/mockgen -typed -package removal -destination package_mocks_test.go github.com/juju/juju/internal/worker/removal RemovalService,Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package removal -destination timer_mocks_test.go github.com/juju/clock Timer
