// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/machine/service State,StatusHistory
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination provider_mock_test.go github.com/juju/juju/domain/machine/service Provider
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination migration_mock_test.go github.com/juju/juju/domain/machine/service MigrationState
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination constraints_mock_test.go github.com/juju/juju/core/constraints Validator
