// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationimportreconciler

//go:generate go run github.com/canonical/gomock/mockgen -package migrationimportreconciler -destination service_mock_test.go github.com/juju/juju/internal/worker/migrationimportreconciler Service
