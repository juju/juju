// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioning -destination provider_mock_test.go github.com/juju/juju/domain/storageprovisioning StorageProvider
