// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

//go:generate go run github.com/canonical/gomock/mockgen -package store -destination store_mock_test.go github.com/juju/juju/core/objectstore ObjectStore,ModelObjectStoreGetter
