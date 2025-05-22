// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

//go:generate go run go.uber.org/mock/mockgen -typed -package lease -destination lease_mock_test.go github.com/juju/juju/core/lease Secretary
