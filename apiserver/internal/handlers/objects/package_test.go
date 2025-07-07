// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

//go:generate go run go.uber.org/mock/mockgen -typed -package objects -destination service_mock_test.go github.com/juju/juju/apiserver/internal/handlers/objects ApplicationServiceGetter,ApplicationService,ObjectStoreServiceGetter,ObjectStoreService
