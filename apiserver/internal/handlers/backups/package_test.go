// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

//go:generate go run github.com/canonical/gomock/mockgen -package backups -destination backupcreator_mock_test.go github.com/juju/juju/core/backups BackupCreator
