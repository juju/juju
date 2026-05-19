// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

//go:generate go run github.com/canonical/gomock/mockgen -package migration -destination package_mock_test.go github.com/juju/juju/domain/model/service/migration State
