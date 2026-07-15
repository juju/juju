// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationreconciler

//go:generate go run github.com/canonical/gomock/mockgen -package migrationreconciler -destination service_mock_test.go github.com/juju/juju/internal/worker/migrationreconciler Service
