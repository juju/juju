// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioning -destination provider_mock_test.go github.com/juju/juju/domain/storageprovisioning StorageProvider
